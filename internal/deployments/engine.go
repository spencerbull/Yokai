package deployments

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
)

type Engine struct {
	Store             *Store
	Ops               Operations
	Now               func() time.Time
	NewID             func() string
	ReadinessTimeout  time.Duration
	ReadinessInterval time.Duration
	CleanupTimeout    time.Duration
	mu                sync.Mutex
}

type CreateOutcome struct {
	Deployment Deployment
	Created    bool
}

func NewEngine(store *Store, ops Operations) *Engine {
	return &Engine{Store: store, Ops: ops, Now: time.Now, NewID: randomDeploymentID, ReadinessTimeout: DefaultReadinessTimeout, ReadinessInterval: DefaultReadinessInterval, CleanupTimeout: DefaultCleanupTimeout}
}

func (e *Engine) Create(ctx context.Context, request CreateRequest) (Deployment, error) {
	outcome, err := e.CreateWithOutcome(ctx, request)
	return outcome.Deployment, err
}

func (e *Engine) CreateWithOutcome(ctx context.Context, request CreateRequest) (CreateOutcome, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	request = normalizeRequest(request)
	cfg, err := validateCreateRequest(request)
	if err != nil {
		return CreateOutcome{}, WrapError(ErrorValidation, "validate deployment", err)
	}
	hash, err := requestHash(request)
	if err != nil {
		return CreateOutcome{}, WrapError(ErrorValidation, "hash deployment request", err)
	}
	if prior, ok := e.Store.FindIdempotency(request.IdempotencyKey); ok {
		if prior.RequestHash != hash {
			return CreateOutcome{}, WrapError(ErrorConflict, "replay deployment", ErrIdempotencyConflict)
		}
		switch prior.State {
		case StateRunning, StateStopped, StateRolledBack:
			return CreateOutcome{Deployment: prior}, nil
		case StatePending, StateRollbackFailed:
			reconciled, recoveryErr := e.rollbackLocked(ctx, prior, "incomplete idempotent request recovered", "")
			if recoveryErr != nil {
				return CreateOutcome{Deployment: reconciled}, recoveryErr
			}
			return CreateOutcome{Deployment: reconciled}, WrapError(ErrorConflict, "recover deployment", ErrRecoveryPerformed)
		case StateStarting, StateStopping, StateFailed:
			reconciled, recoveryErr := e.reconcileUnsafeStateLocked(ctx, prior)
			if recoveryErr != nil {
				return CreateOutcome{Deployment: reconciled}, recoveryErr
			}
			return CreateOutcome{Deployment: reconciled}, WrapError(ErrorConflict, "recover deployment lifecycle", ErrRecoveryPerformed)
		default:
			return CreateOutcome{}, WrapError(ErrorConflict, "replay deployment", fmt.Errorf("deployment has unsupported state %q", prior.State))
		}
	}

	ordered := orderedBindings(request.Bindings, cfg)
	deploymentID := e.NewID()
	previous := make([]Member, 0, len(ordered))
	previousGeneration := 0
	for index := range ordered {
		binding := &ordered[index]
		if binding.ObservedContainerID == "" {
			continue
		}
		observed, inspectErr := e.Ops.Inspect(ctx, binding.DeviceID, binding.ObservedContainerID)
		if inspectErr != nil {
			kind := ErrorKindOf(inspectErr)
			if kind == "" {
				kind = ErrorValidation
			}
			return CreateOutcome{}, WrapError(kind, "inspect selected "+binding.Role+" container", inspectErr)
		}
		if observed.DeploymentID != "" {
			return CreateOutcome{}, WrapError(ErrorConflict, "inspect selected "+binding.Role+" container", fmt.Errorf("container is already managed by grouped deployment %s", observed.DeploymentID))
		}
		binding.ObservedContainerID = observed.ID
		member := memberFromObserved(*binding, roleRank(cfg, binding.Role), observed)
		previous = append(previous, member)
		if member.Generation > previousGeneration {
			previousGeneration = member.Generation
		}
	}

	now := e.Now().UTC()
	deployment := Deployment{
		ID: deploymentID, BKCID: cfg.ID, IdempotencyKey: request.IdempotencyKey, RequestHash: hash,
		State: StatePending, Phase: PhasePlanned, Generation: previousGeneration + 1, PreviousGeneration: previousGeneration,
		Bindings: cloneBindings(ordered), PreviousMembers: previous, UsesLocalModelSnapshot: request.LocalModelPath != "",
		RuntimePatches: runtimePatchProvenance(cfg),
		CreatedAt:      now, UpdatedAt: now,
	}
	for _, binding := range ordered {
		candidate := buildCandidate(deployment, request, cfg, binding, ordered[0].FabricAddress)
		deployment.Members = append(deployment.Members, plannedMember(binding, candidate, deployment.Generation))
	}

	for index, binding := range ordered {
		preflight := PreflightRequest{
			Binding: binding, CandidateName: deployment.Members[index].Name, LocalModelPath: request.LocalModelPath,
			ServicePort: cfg.MultiDevice.ServicePort, RendezvousPort: cfg.MultiDevice.RendezvousPort, Head: binding.Role == bkc.MultiDeviceRoleHead,
		}
		if preflightErr := e.Ops.Preflight(ctx, preflight, cfg.MultiDevice.RequiredCapabilities, cfg.TargetDevices); preflightErr != nil {
			kind := ErrorKindOf(preflightErr)
			if kind == "" {
				kind = ErrorConflict
			}
			return CreateOutcome{}, WrapError(kind, "preflight "+binding.Role, preflightErr)
		}
	}

	if err := e.persistProgress(&deployment, PhasePlanned, "journal", "", "completed", "candidate and previous members selected"); err != nil {
		return CreateOutcome{}, WrapError(ErrorDependency, "journal pending deployment", err)
	}
	for index, binding := range ordered {
		if err := e.beforeMemberAction(&deployment, index, PhasePulling, "pull", "pulling"); err != nil {
			return e.createRollback(ctx, deployment, err, "persist pull intent failed", request.APIKey)
		}
		if err := e.Ops.Pull(ctx, binding, cfg.Image); err != nil {
			return e.createRollback(ctx, deployment, fmt.Errorf("pull %s candidate: %w", binding.Role, err), "candidate pull failed", request.APIKey)
		}
		if err := e.afterMemberAction(&deployment, index, PhasePulling, "pull", "pulled"); err != nil {
			return e.createRollback(ctx, deployment, err, "persist pull completion failed", request.APIKey)
		}
	}
	for index := range deployment.PreviousMembers {
		member := deployment.PreviousMembers[index]
		if err := e.beforePreviousAction(&deployment, index, PhaseCutover, "stop_previous", "stopping"); err != nil {
			return e.createRollback(ctx, deployment, err, "persist previous stop intent failed", request.APIKey)
		}
		if err := e.Ops.Stop(ctx, member.DeviceID, member.ContainerID); err != nil {
			return e.createRollback(ctx, deployment, fmt.Errorf("stop selected %s container: %w", member.Role, err), "selected previous container stop failed", request.APIKey)
		}
		if err := e.afterPreviousAction(&deployment, index, PhaseCutover, "stop_previous", "stopped"); err != nil {
			return e.createRollback(ctx, deployment, err, "persist previous stop completion failed", request.APIKey)
		}
	}
	for index, binding := range ordered {
		candidate := buildCandidate(deployment, request, cfg, binding, ordered[0].FabricAddress)
		if err := e.beforeMemberAction(&deployment, index, PhaseLaunching, "launch", "launching"); err != nil {
			return e.createRollback(ctx, deployment, err, "persist launch intent failed", request.APIKey)
		}
		observed, launchErr := e.Ops.Launch(ctx, binding, candidate)
		if launchErr != nil {
			return e.createRollback(ctx, deployment, fmt.Errorf("launch %s candidate: %w", binding.Role, launchErr), "candidate launch failed", request.APIKey)
		}
		deployment.Members[index].ContainerID = observed.ID
		deployment.Members[index].Name = observed.Name
		deployment.Members[index].Ownership = OwnershipManaged
		if err := e.afterMemberAction(&deployment, index, PhaseLaunching, "launch", observed.Status); err != nil {
			return e.createRollback(ctx, deployment, err, "persist launch completion failed", request.APIKey)
		}
		if requestErr := ctx.Err(); requestErr != nil {
			return e.createRollback(ctx, deployment, requestErr, "deployment request ended after candidate launch", request.APIKey)
		}
	}
	if err := e.persistProgress(&deployment, PhaseReadiness, "readiness", "", "started", "waiting for both ranks and semantic gates"); err != nil {
		return e.createRollback(ctx, deployment, err, "persist readiness intent failed", request.APIKey)
	}
	readyMembers, testResult, readyErr := e.waitReady(ctx, deployment.Members, request.APIKey, expectedModel(cfg, request.LocalModelPath != ""))
	deployment.Members = readyMembers
	if readyErr != nil {
		return e.createRollback(ctx, deployment, readinessOperationError(readyErr), "candidate readiness failed", request.APIKey)
	}
	deployment.LastTest = &testResult
	if err := e.persistProgress(&deployment, PhaseReadiness, "readiness", "", "completed", "both ranks running and semantic gates passed"); err != nil {
		return e.createRollback(ctx, deployment, err, "persist readiness completion failed", request.APIKey)
	}
	deployment.State = StateRunning
	if err := e.persistProgress(&deployment, PhaseRunning, "promote", "", "completed", "both ranks running and semantic gates passed"); err != nil {
		return e.createRollback(ctx, deployment, err, "persist promotion failed", request.APIKey)
	}
	return CreateOutcome{Deployment: deployment, Created: true}, nil
}

func (e *Engine) createRollback(ctx context.Context, deployment Deployment, cause error, safeCause, exactRedaction string) (CreateOutcome, error) {
	// A client deadline or disconnect may be the reason launch was ambiguous.
	// Cleanup must still get a bounded chance to inspect deterministic names and
	// restore selected prior members; it must not inherit a canceled request.
	rollbackCtx, cancel := e.boundedCleanupContext(ctx)
	defer cancel()
	rolledBack, rollbackErr := e.rollbackLocked(rollbackCtx, deployment, safeCause, exactRedaction)
	if rollbackErr != nil {
		return CreateOutcome{Deployment: rolledBack}, fmt.Errorf("%w; %v", cause, rollbackErr)
	}
	return CreateOutcome{Deployment: rolledBack}, cause
}

func (e *Engine) List() []Deployment { return e.Store.List() }

func (e *Engine) Get(id string) (Deployment, error) { return e.Store.Get(id) }

func (e *Engine) Test(ctx context.Context, id, apiKey string) (Deployment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := validateAPIKey(apiKey); err != nil {
		return Deployment{}, WrapError(ErrorValidation, "validate api key", err)
	}
	deployment, err := e.Store.Get(id)
	if err != nil {
		return Deployment{}, err
	}
	head, ok := memberByRole(deployment.Members, bkc.MultiDeviceRoleHead)
	if !ok {
		return Deployment{}, WrapError(ErrorConflict, "test deployment", fmt.Errorf("deployment has no rank-0 member"))
	}
	result, err := e.Ops.Test(ctx, head.DeviceID, memberLocator(head), apiKey)
	if err != nil {
		return Deployment{}, err
	}
	expected, err := expectedStoredDeploymentModel(deployment)
	if err != nil {
		return Deployment{}, err
	}
	if err := validatePromotionResult(result, expected); err != nil {
		return Deployment{}, WrapError(ErrorDependency, "test rank-0 API/model/metrics", err)
	}
	result.TestedAt = e.Now().UTC()
	deployment.LastTest = &result
	deployment.UpdatedAt = result.TestedAt
	if err := e.Store.Put(deployment); err != nil {
		return Deployment{}, err
	}
	return deployment, nil
}

func (e *Engine) Stop(ctx context.Context, id string) (Deployment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	deployment, err := e.Store.Get(id)
	if err != nil {
		return Deployment{}, err
	}
	return e.stopLocked(ctx, deployment)
}

func (e *Engine) stopLocked(ctx context.Context, deployment Deployment) (Deployment, error) {
	if deployment.State == StateStopped {
		return deployment, nil
	}
	if deployment.State != StateRunning && deployment.State != StateStopping && deployment.State != StateStarting && deployment.State != StateFailed {
		return Deployment{}, WrapError(ErrorConflict, "stop deployment", fmt.Errorf("deployment state %s cannot be stopped", deployment.State))
	}
	deployment.State = StateStopping
	if err := e.persistProgress(&deployment, PhaseStopping, "stop_group", "", "started", "worker then head"); err != nil {
		return Deployment{}, err
	}
	var stopErrors []string
	for index := len(deployment.Members) - 1; index >= 0; index-- {
		member := deployment.Members[index]
		if member.Ownership == OwnershipObserved {
			stopErrors = append(stopErrors, "refused observed "+member.Role)
			continue
		}
		if err := e.beforeMemberAction(&deployment, index, PhaseStopping, "stop", "stopping"); err != nil {
			stopErrors = append(stopErrors, "persist "+member.Role+" stop intent")
			continue
		}
		observed, inspectErr := e.Ops.Inspect(ctx, member.DeviceID, memberLocator(member))
		if inspectErr != nil {
			stopErrors = append(stopErrors, "inspect "+member.Role)
			continue
		}
		if err := validateManagedMemberIdentity(deployment, member, observed); err != nil {
			stopErrors = append(stopErrors, "provenance "+member.Role)
			continue
		}
		if observed.Status != "stopped" {
			if err := e.Ops.StopManaged(ctx, deployment, member); err != nil {
				stopErrors = append(stopErrors, "stop "+member.Role)
				continue
			}
		}
		if err := e.afterMemberAction(&deployment, index, PhaseStopping, "stop", "stopped"); err != nil {
			stopErrors = append(stopErrors, "persist "+member.Role+" stop completion")
		}
	}
	if len(stopErrors) > 0 {
		deployment.State = StateFailed
		deployment.Error = "group stop incomplete: " + strings.Join(stopErrors, ", ")
		if err := e.persistProgress(&deployment, PhaseComplete, "stop_group", "", "failed", deployment.Error); err != nil {
			return deployment, err
		}
		return deployment, WrapError(ErrorConflict, "stop deployment", fmt.Errorf("%s", deployment.Error))
	}
	deployment.State = StateStopped
	if err := e.persistProgress(&deployment, PhaseComplete, "stop_group", "", "completed", "all managed members stopped"); err != nil {
		return Deployment{}, err
	}
	return deployment, nil
}

func (e *Engine) Start(ctx context.Context, id, apiKey string) (Deployment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := validateAPIKey(apiKey); err != nil {
		return Deployment{}, WrapError(ErrorValidation, "validate api key", err)
	}
	deployment, err := e.Store.Get(id)
	if err != nil {
		return Deployment{}, err
	}
	if deployment.State == StateRunning {
		needsRecovery := false
		for index := range deployment.Members {
			member := deployment.Members[index]
			observed, inspectErr := e.Ops.Inspect(ctx, member.DeviceID, memberLocator(member))
			if inspectErr != nil {
				return e.markUnsafeRunning(deployment, fmt.Errorf("inspect %s rank: %w", member.Role, inspectErr))
			}
			if identityErr := validateManagedMemberIdentity(deployment, member, observed); identityErr != nil {
				return e.markUnsafeRunning(deployment, identityErr)
			}
			deployment.Members[index].Status = observed.Status
			switch observed.Status {
			case "running":
			case "stopped", "exited", "dead":
				needsRecovery = true
			default:
				return e.markUnsafeRunning(deployment, fmt.Errorf("managed %s rank is %s", member.Role, observed.Status))
			}
		}
		if !needsRecovery {
			return deployment, nil
		}
		recovered, cleanupErr := e.failStart(ctx, deployment, WrapError(ErrorConflict, "verify running deployment", fmt.Errorf("one or more managed ranks exited")))
		if recovered.State != StateStopped {
			return recovered, cleanupErr
		}
		deployment = recovered
	}
	if deployment.State != StateStopped && deployment.State != StateStarting {
		return Deployment{}, WrapError(ErrorConflict, "start deployment", fmt.Errorf("deployment state %s cannot be started", deployment.State))
	}
	deployment.State = StateStarting
	deployment.Error = ""
	if err := e.persistProgress(&deployment, PhaseStarting, "start_group", "", "started", "head then worker"); err != nil {
		return Deployment{}, err
	}
	for _, role := range []string{bkc.MultiDeviceRoleHead, bkc.MultiDeviceRoleWorker} {
		index := memberIndexByRole(deployment.Members, role)
		if index < 0 {
			return Deployment{}, WrapError(ErrorConflict, "start deployment", fmt.Errorf("deployment has no %s member", role))
		}
		member := deployment.Members[index]
		if err := e.beforeMemberAction(&deployment, index, PhaseStarting, "restart", "starting"); err != nil {
			return Deployment{}, err
		}
		observed, inspectErr := e.Ops.Inspect(ctx, member.DeviceID, memberLocator(member))
		if inspectErr != nil {
			if errors.Is(inspectErr, ErrContainerNotFound) {
				return e.failStart(ctx, deployment, WrapError(ErrorConflict, "start deployment", fmt.Errorf("managed %s member is absent", member.Role)))
			}
			return Deployment{}, inspectErr
		}
		if identityErr := validateManagedMemberIdentity(deployment, member, observed); identityErr != nil {
			return e.failStart(ctx, deployment, WrapError(ErrorConflict, "start deployment", identityErr))
		}
		if observed.Status != "running" || member.Status == "stopping" {
			if err := e.Ops.RestartManaged(ctx, deployment, member); err != nil {
				return e.failStart(ctx, deployment, err)
			}
		}
		if err := e.afterMemberAction(&deployment, index, PhaseStarting, "restart", "running"); err != nil {
			return Deployment{}, err
		}
	}
	if err := e.persistProgress(&deployment, PhaseReadiness, "readiness", "", "started", "waiting for both ranks and semantic gates"); err != nil {
		return Deployment{}, err
	}
	expected, err := expectedStoredDeploymentModel(deployment)
	if err != nil {
		return Deployment{}, err
	}
	readyMembers, result, readyErr := e.waitReady(ctx, deployment.Members, apiKey, expected)
	deployment.Members = readyMembers
	if readyErr != nil {
		return e.failStart(ctx, deployment, readinessOperationError(readyErr))
	}
	deployment.LastTest = &result
	if err := e.persistProgress(&deployment, PhaseReadiness, "readiness", "", "completed", "both ranks running and semantic gates passed"); err != nil {
		return Deployment{}, err
	}
	deployment.State = StateRunning
	if err := e.persistProgress(&deployment, PhaseRunning, "start_group", "", "completed", "both ranks running and semantic gates passed"); err != nil {
		return Deployment{}, err
	}
	return deployment, nil
}

func (e *Engine) markUnsafeRunning(deployment Deployment, cause error) (Deployment, error) {
	deployment.State = StateFailed
	deployment.Error = "running deployment member verification failed"
	if err := e.persistProgress(&deployment, PhaseComplete, "verify_running", "", "failed", deployment.Error); err != nil {
		return deployment, fmt.Errorf("%w; persist unsafe running state: %v", cause, err)
	}
	return deployment, WrapError(ErrorConflict, "verify running deployment", cause)
}

func (e *Engine) failStart(ctx context.Context, deployment Deployment, cause error) (Deployment, error) {
	cleanupCtx, cancel := e.boundedCleanupContext(ctx)
	defer cancel()
	var stopErrors []string
	for index := len(deployment.Members) - 1; index >= 0; index-- {
		member := deployment.Members[index]
		observed, inspectErr := e.Ops.Inspect(cleanupCtx, member.DeviceID, memberLocator(member))
		if inspectErr != nil || validateManagedMemberIdentity(deployment, member, observed) != nil {
			stopErrors = append(stopErrors, member.Role)
			continue
		}
		if observed.Status != "stopped" && observed.Status != "exited" && observed.Status != "dead" {
			if err := e.Ops.StopManaged(cleanupCtx, deployment, member); err != nil {
				stopErrors = append(stopErrors, member.Role)
				continue
			}
		}
		deployment.Members[index].Status = "stopped"
	}
	deployment.Error = "group start readiness failed"
	deployment.State = StateStopped
	if len(stopErrors) > 0 {
		deployment.State = StateFailed
		deployment.Error = "group start failed and one or more members could not be stopped"
	}
	if err := e.persistProgress(&deployment, PhaseComplete, "start_group", "", "failed", deployment.Error); err != nil {
		return deployment, fmt.Errorf("%w; persist failed start: %v", cause, err)
	}
	if len(stopErrors) > 0 {
		return deployment, fmt.Errorf("%w; failed to stop roles %s", cause, strings.Join(stopErrors, ","))
	}
	return deployment, cause
}

func (e *Engine) boundedCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := e.CleanupTimeout
	if timeout <= 0 {
		timeout = DefaultCleanupTimeout
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func (e *Engine) Rollback(ctx context.Context, id string) (Deployment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	deployment, err := e.Store.Get(id)
	if err != nil {
		return Deployment{}, err
	}
	if deployment.State == StateRolledBack {
		return deployment, nil
	}
	return e.rollbackLocked(ctx, deployment, "rollback requested", "")
}

func (e *Engine) Reconcile(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var joined []error
	for _, deployment := range e.Store.List() {
		var err error
		switch deployment.State {
		case StatePending, StateRollbackFailed:
			_, err = e.rollbackLocked(ctx, deployment, "daemon restart reconciliation", "")
		case StateStarting, StateStopping, StateFailed:
			_, err = e.reconcileUnsafeStateLocked(ctx, deployment)
		default:
			continue
		}
		if err != nil {
			joined = append(joined, fmt.Errorf("reconcile %s: %w", deployment.ID, err))
		}
	}
	return errors.Join(joined...)
}

type unsafeStateRecovery int

const (
	recoverIncompleteCreate unsafeStateRecovery = iota
	recoverInterruptedStart
	recoverInterruptedStop
	recoverFailedRunningVerification
)

// unsafeStateRecoveryFor uses the latest durable lifecycle intent, rather than
// StateFailed alone, to distinguish an incomplete create from later operations
// on an already-promoted group. This is also the authority for idempotent create
// replay, so replaying the original key cannot turn a failed Start, Stop, or
// running verification into a destructive create rollback.
func unsafeStateRecoveryFor(deployment Deployment) unsafeStateRecovery {
	// The explicit nonterminal state wins over later subphase progress. Start
	// persists StateStarting before it enters readiness, so a crash can leave a
	// latest readiness record that still belongs to the post-promotion Start
	// lifecycle and must never be mistaken for create-time readiness.
	switch deployment.State {
	case StateStarting:
		return recoverInterruptedStart
	case StateStopping:
		return recoverInterruptedStop
	}
	for index := len(deployment.Progress) - 1; index >= 0; index-- {
		switch deployment.Progress[index].Action {
		case "start_group":
			return recoverInterruptedStart
		case "stop_group":
			return recoverInterruptedStop
		case "verify_running":
			return recoverFailedRunningVerification
		case "rollback", "promote", "readiness", "launch", "stop_previous", "pull", "journal":
			return recoverIncompleteCreate
		}
	}
	switch deployment.Phase {
	case PhaseStarting:
		return recoverInterruptedStart
	case PhaseStopping:
		return recoverInterruptedStop
	default:
		return recoverIncompleteCreate
	}
}

func (e *Engine) reconcileUnsafeStateLocked(ctx context.Context, deployment Deployment) (Deployment, error) {
	cleanupCtx, cancel := e.boundedCleanupContext(ctx)
	defer cancel()
	switch unsafeStateRecoveryFor(deployment) {
	case recoverInterruptedStart:
		recovered, cleanupErr := e.failStart(cleanupCtx, deployment, WrapError(ErrorConflict, "reconcile start", ErrRecoveryPerformed))
		if recovered.State == StateStopped {
			return recovered, nil
		}
		return recovered, cleanupErr
	case recoverInterruptedStop, recoverFailedRunningVerification:
		// A failed provenance/availability check is handled by stopLocked's
		// fail-closed member validation. It may stop other exact managed ranks,
		// but it never removes candidates or restores PreviousMembers.
		return e.stopLocked(cleanupCtx, deployment)
	default:
		return e.rollbackLocked(cleanupCtx, deployment, "daemon restart reconciliation", "")
	}
}

func (e *Engine) rollbackLocked(ctx context.Context, deployment Deployment, safeCause, exactRedaction string) (Deployment, error) {
	result := &RollbackResult{Attempted: true}
	if deployment.Rollback != nil {
		result.RemovedCandidates = appendUniqueStrings(result.RemovedCandidates, deployment.Rollback.RemovedCandidates...)
		result.RestartedContainers = appendUniqueStrings(result.RestartedContainers, deployment.Rollback.RestartedContainers...)
		result.LogTails = append([]RankLogTail(nil), deployment.Rollback.LogTails...)
	}
	deployment.Rollback = result
	deployment.State = StateRollbackFailed
	deployment.Error = safeCause
	if err := e.persistProgress(&deployment, PhaseRollback, "rollback", "", "started", "remove candidates then restore previous members"); err != nil {
		return deployment, WrapError(ErrorDependency, "persist rollback intent", err)
	}

	candidatesSafe := true
	for index := len(deployment.Members) - 1; index >= 0; index-- {
		member := deployment.Members[index]
		if !candidateMayExist(member.Status) {
			continue
		}
		if member.Ownership == OwnershipObserved {
			result.Errors = append(result.Errors, "refused to remove observed "+member.Role+" candidate")
			candidatesSafe = false
			continue
		}
		if err := e.beforeMemberAction(&deployment, index, PhaseRollback, "remove_candidate", "removing"); err != nil {
			result.Errors = append(result.Errors, "persist "+member.Role+" candidate removal intent failed")
			candidatesSafe = false
			continue
		}
		observed, inspectErr := e.Ops.Inspect(ctx, member.DeviceID, member.Name)
		if inspectErr != nil && !errors.Is(inspectErr, ErrContainerNotFound) {
			result.Errors = append(result.Errors, "inspect "+member.Role+" candidate failed")
			candidatesSafe = false
			continue
		}
		if inspectErr == nil {
			if err := validateManagedMemberIdentity(deployment, member, observed); err != nil {
				result.Errors = append(result.Errors, "candidate name collision for "+member.Role)
				candidatesSafe = false
				continue
			}
			member.ContainerID = observed.ID
		}
		memberRedaction := ""
		if member.Role == bkc.MultiDeviceRoleHead {
			memberRedaction = exactRedaction
		}
		capture, captureErr := e.Ops.CaptureManagedLogs(ctx, deployment, member, memberRedaction)
		logTail := RankLogTail{
			Role: member.Role, Rank: member.Rank, DeviceID: member.DeviceID,
			ContainerID: member.ContainerID, Name: member.Name, CapturedAt: e.Now().UTC(),
		}
		if captureErr != nil {
			logTail.Status = "failed"
			logTail.Error, _ = SanitizeLogTail(captureErr.Error(), exactRedaction)
		} else {
			logTail.Status = "captured"
			logTail.Tail, logTail.Truncated = SanitizeLogTail(capture.Tail, exactRedaction)
			logTail.Truncated = logTail.Truncated || capture.Truncated
		}
		result.LogTails = upsertRankLogTail(result.LogTails, logTail)
		deployment.Rollback = result
		captureStatus := logTail.Status
		captureDetail := "bounded candidate log tail captured"
		if captureErr != nil {
			captureDetail = "bounded candidate log tail capture failed; removal will continue"
		}
		// Log capture is diagnostic and best effort. A capture or journal failure
		// must never block the cleanup barrier or previous-service restoration.
		_ = e.persistProgress(&deployment, PhaseRollback, "capture_candidate_logs", member.Role, captureStatus, captureDetail)
		// Always cross the provenance-aware agent deletion endpoint, including
		// when the preceding read observed absence. That endpoint is also the
		// completion barrier for an in-flight ambiguous docker run of this exact
		// deterministic name. It returns not-found only after that launch and its
		// owned cleanup have finished.
		if err := e.Ops.RemoveManaged(ctx, deployment, member); err != nil && !errors.Is(err, ErrContainerNotFound) {
			result.Errors = append(result.Errors, "remove "+member.Role+" candidate failed")
			candidatesSafe = false
			continue
		}
		result.RemovedCandidates = appendUniqueStrings(result.RemovedCandidates, member.Name)
		deployment.Rollback = result
		if err := e.afterMemberAction(&deployment, index, PhaseRollback, "remove_candidate", "removed"); err != nil {
			result.Errors = append(result.Errors, "persist "+member.Role+" candidate removal failed")
		}
	}

	// Previous members are restored in service dependency order: head, worker,
	// but only after every candidate is known absent. This prevents an ambiguous
	// launch that outlives the rollback context from racing a restored service.
	for index := range deployment.PreviousMembers {
		if !candidatesSafe {
			break
		}
		member := deployment.PreviousMembers[index]
		if err := e.beforePreviousAction(&deployment, index, PhaseRollback, "restore_previous", "restarting"); err != nil {
			result.Errors = append(result.Errors, "persist "+member.Role+" restoration intent failed")
			continue
		}
		observed, inspectErr := e.Ops.Inspect(ctx, member.DeviceID, member.ContainerID)
		if inspectErr != nil {
			result.Errors = append(result.Errors, "inspect previous "+member.Role+" failed")
			continue
		}
		// A durable stopping intent means the stop request may still be in
		// flight even when this first inspect catches the container running.
		// Restart through the agent in that case; its per-container stop barrier
		// serializes the restart after the accepted stop has fully settled.
		if observed.Status != "running" || member.Status == "stopping" {
			if err := e.Ops.Restart(ctx, member.DeviceID, member.ContainerID); err != nil {
				result.Errors = append(result.Errors, "restart previous "+member.Role+" failed")
				continue
			}
			result.RestartedContainers = appendUniqueStrings(result.RestartedContainers, member.ContainerID)
			deployment.Rollback = result
			if err := e.persistProgress(&deployment, PhaseRollback, "restore_previous_restart", member.Role, "completed", "previous member restart requested"); err != nil {
				result.Errors = append(result.Errors, "persist previous "+member.Role+" restart audit failed")
				continue
			}
		} else if member.Status == "restarting" {
			// A crash can occur after Docker accepted restart but before its audit
			// write. The durable intent plus the observed running state reconstructs
			// the completed effect without issuing it twice.
			result.RestartedContainers = appendUniqueStrings(result.RestartedContainers, member.ContainerID)
			deployment.Rollback = result
			if err := e.persistProgress(&deployment, PhaseRollback, "restore_previous_restart", member.Role, "recovered", "previous member restart completion reconstructed"); err != nil {
				result.Errors = append(result.Errors, "persist previous "+member.Role+" reconstructed restart audit failed")
				continue
			}
		}
		if err := e.Ops.WaitRunning(ctx, member.DeviceID, member.ContainerID); err != nil {
			result.Errors = append(result.Errors, "wait for previous "+member.Role+" failed")
			continue
		}
		deployment.PreviousMembers[index].Status = "running"
		if err := e.persistProgress(&deployment, PhaseRollback, "restore_previous", member.Role, "completed", "previous member is running"); err != nil {
			result.Errors = append(result.Errors, "persist previous "+member.Role+" restoration failed")
		}
	}

	result.CompletedAt = e.Now().UTC()
	deployment.Rollback = result
	deployment.UpdatedAt = result.CompletedAt
	if len(result.Errors) == 0 {
		deployment.State = StateRolledBack
		for index := range deployment.Members {
			if deployment.Members[index].Status != "removed" {
				deployment.Members[index].Status = "stopped"
			}
		}
	} else {
		deployment.State = StateRollbackFailed
	}
	status := "completed"
	if len(result.Errors) > 0 {
		status = "failed"
	}
	if err := e.persistProgress(&deployment, PhaseComplete, "rollback", "", status, "candidate cleanup and previous restoration checked"); err != nil {
		return deployment, WrapError(ErrorDependency, "persist rollback result", err)
	}
	if len(result.Errors) > 0 {
		return deployment, WrapError(ErrorDependency, "rollback deployment", fmt.Errorf("%s", strings.Join(result.Errors, "; ")))
	}
	return deployment, nil
}

func validateManagedMemberIdentity(deployment Deployment, member Member, observed ObservedContainer) error {
	if member.Ownership != OwnershipManaged || !observed.Managed || observed.Ownership != OwnershipManaged {
		return fmt.Errorf("%s member is not managed by Yokai", member.Role)
	}
	if observed.DeploymentID != deployment.ID || observed.Generation != member.Generation || observed.Role != member.Role {
		return fmt.Errorf("%s member provenance does not match deployment", member.Role)
	}
	if strings.TrimSpace(member.Name) == "" || observed.Name != member.Name {
		return fmt.Errorf("%s member deterministic name does not match deployment", member.Role)
	}
	return nil
}

func appendUniqueStrings(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		existing = append(existing, value)
		seen[value] = struct{}{}
	}
	return existing
}

func upsertRankLogTail(existing []RankLogTail, value RankLogTail) []RankLogTail {
	for index := range existing {
		if existing[index].Role == value.Role && existing[index].Rank == value.Rank {
			// A retry can observe that removal already succeeded and therefore be
			// unable to recapture logs. Preserve the useful durable evidence from
			// the earlier attempt; any later successful capture may still replace it.
			if existing[index].Status == "captured" && value.Status == "failed" {
				return existing
			}
			existing[index] = value
			return existing
		}
	}
	return append(existing, value)
}

func (e *Engine) waitReady(ctx context.Context, members []Member, apiKey, expectedModelID string) ([]Member, TestResult, error) {
	timeout := e.ReadinessTimeout
	if timeout <= 0 {
		timeout = DefaultReadinessTimeout
	}
	interval := e.ReadinessInterval
	if interval <= 0 {
		interval = DefaultReadinessInterval
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var lastErr error
	for {
		allRunning := true
		for index := range members {
			observed, err := e.Ops.Inspect(ctx, members[index].DeviceID, memberLocator(members[index]))
			if err != nil {
				allRunning = false
				lastErr = fmt.Errorf("inspect %s rank during readiness: %w", members[index].Role, err)
				continue
			}
			members[index].Status = observed.Status
			if observed.ID != "" {
				members[index].ContainerID = observed.ID
			}
			if observed.Name != "" {
				members[index].Name = observed.Name
			}
			switch observed.Status {
			case "running":
			case "stopped", "exited", "dead":
				return members, TestResult{}, fmt.Errorf("%s rank exited during readiness", members[index].Role)
			default:
				allRunning = false
				lastErr = fmt.Errorf("%s rank is %s", members[index].Role, observed.Status)
			}
		}
		if allRunning {
			head, ok := memberByRole(members, bkc.MultiDeviceRoleHead)
			if !ok {
				return members, TestResult{}, fmt.Errorf("deployment has no rank-0 member")
			}
			result, err := e.Ops.Test(ctx, head.DeviceID, memberLocator(head), apiKey)
			if err == nil {
				err = validatePromotionResult(result, expectedModelID)
			}
			if err == nil {
				result.TestedAt = e.Now().UTC()
				return members, result, nil
			}
			lastErr = err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return members, TestResult{}, ctx.Err()
		case <-deadline.C:
			stopTimer(timer)
			if lastErr == nil {
				lastErr = fmt.Errorf("readiness deadline exceeded")
			}
			return members, TestResult{}, fmt.Errorf("readiness deadline exceeded: %w", lastErr)
		case <-timer.C:
		}
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func validatePromotionResult(result TestResult, expectedModelID string) error {
	if !result.OK {
		return fmt.Errorf("rank-0 semantic API check did not report success")
	}
	if strings.TrimSpace(result.Model) == "" {
		return fmt.Errorf("rank-0 model discovery returned no model")
	}
	if strings.TrimSpace(result.Model) != expectedModelID {
		return fmt.Errorf("rank-0 model discovery returned unexpected model %q", strings.TrimSpace(result.Model))
	}
	if !result.MetricsReady {
		return fmt.Errorf("rank-0 metrics check did not report ready")
	}
	if !strings.EqualFold(strings.TrimSpace(result.Response), "ok") {
		return fmt.Errorf("rank-0 response content was not exactly ok")
	}
	return nil
}

func expectedStoredDeploymentModel(deployment Deployment) (string, error) {
	cfg, ok := bkc.LookupID(deployment.BKCID)
	if !ok {
		return "", WrapError(ErrorConflict, "resolve deployment recipe", fmt.Errorf("BKC %s is no longer available", deployment.BKCID))
	}
	return expectedModel(cfg, deployment.UsesLocalModelSnapshot), nil
}

func expectedModel(cfg bkc.Config, usesLocalSnapshot bool) string {
	if usesLocalSnapshot {
		return FixedLocalModelPath
	}
	return cfg.ModelID
}

func readinessOperationError(err error) error {
	if err == nil || ErrorKindOf(err) != "" {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return WrapError(ErrorUnavailable, "wait for deployment readiness", err)
	}
	return WrapError(ErrorDependency, "wait for deployment readiness", err)
}

func (e *Engine) persistProgress(deployment *Deployment, phase Phase, action, role, status, detail string) error {
	now := e.Now().UTC()
	deployment.Phase = phase
	deployment.UpdatedAt = now
	deployment.Progress = append(deployment.Progress, Progress{Phase: phase, Action: action, Role: role, Status: status, Detail: detail, At: now})
	return e.Store.Put(*deployment)
}

func (e *Engine) beforeMemberAction(deployment *Deployment, index int, phase Phase, action, status string) error {
	deployment.Members[index].Status = status
	return e.persistProgress(deployment, phase, action, deployment.Members[index].Role, "started", "")
}

func (e *Engine) afterMemberAction(deployment *Deployment, index int, phase Phase, action, status string) error {
	deployment.Members[index].Status = status
	return e.persistProgress(deployment, phase, action, deployment.Members[index].Role, "completed", "")
}

func (e *Engine) beforePreviousAction(deployment *Deployment, index int, phase Phase, action, status string) error {
	deployment.PreviousMembers[index].Status = status
	return e.persistProgress(deployment, phase, action, deployment.PreviousMembers[index].Role, "started", "")
}

func (e *Engine) afterPreviousAction(deployment *Deployment, index int, phase Phase, action, status string) error {
	deployment.PreviousMembers[index].Status = status
	return e.persistProgress(deployment, phase, action, deployment.PreviousMembers[index].Role, "completed", "")
}

func validateCreateRequest(request CreateRequest) (bkc.Config, error) {
	if request.BKCID == "" {
		return bkc.Config{}, fmt.Errorf("bkc_id is required")
	}
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 {
		return bkc.Config{}, fmt.Errorf("idempotency_key must be 1-128 characters")
	}
	if strings.IndexFunc(request.IdempotencyKey, unicode.IsControl) >= 0 {
		return bkc.Config{}, fmt.Errorf("idempotency_key contains control characters")
	}
	if err := validateAPIKey(request.APIKey); err != nil {
		return bkc.Config{}, err
	}
	if request.LocalModelPath != "" {
		clean := filepath.Clean(request.LocalModelPath)
		if !filepath.IsAbs(clean) || clean == string(filepath.Separator) || clean != request.LocalModelPath {
			return bkc.Config{}, fmt.Errorf("local_model_path must be an explicit clean absolute path")
		}
	}
	cfg, ok := bkc.LookupID(request.BKCID)
	if !ok {
		return bkc.Config{}, fmt.Errorf("unknown BKC %q", request.BKCID)
	}
	if err := bkc.ValidateMultiDeviceRecipe(cfg); err != nil {
		return bkc.Config{}, err
	}
	if len(request.Bindings) != len(cfg.MultiDevice.Roles) {
		return bkc.Config{}, fmt.Errorf("exactly %d role bindings are required", len(cfg.MultiDevice.Roles))
	}
	roles := make(map[string]Binding, len(request.Bindings))
	devices := make(map[string]struct{}, len(request.Bindings))
	addresses := make(map[string]struct{}, len(request.Bindings))
	serviceAddress := ""
	for _, binding := range request.Bindings {
		if binding.Role == "" || binding.DeviceID == "" || binding.FabricAddress == "" {
			return bkc.Config{}, fmt.Errorf("every binding requires role, device_id, and fabric_address")
		}
		if _, duplicate := roles[binding.Role]; duplicate {
			return bkc.Config{}, fmt.Errorf("duplicate role %q", binding.Role)
		}
		roles[binding.Role] = binding
		if _, duplicate := devices[binding.DeviceID]; duplicate {
			return bkc.Config{}, fmt.Errorf("role bindings must use two distinct devices")
		}
		devices[binding.DeviceID] = struct{}{}
		ip := net.ParseIP(binding.FabricAddress)
		if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
			return bkc.Config{}, fmt.Errorf("invalid fabric address for role %s", binding.Role)
		}
		canonicalIP := ip.String()
		if _, duplicate := addresses[canonicalIP]; duplicate {
			return bkc.Config{}, fmt.Errorf("role bindings must use distinct fabric addresses")
		}
		addresses[canonicalIP] = struct{}{}
		if binding.Role == bkc.MultiDeviceRoleHead {
			serviceIP := net.ParseIP(binding.ServiceAddress)
			if serviceIP == nil || serviceIP.IsUnspecified() || serviceIP.IsLoopback() || serviceIP.IsMulticast() {
				return bkc.Config{}, fmt.Errorf("head binding requires an explicit valid service_address")
			}
			if binding.ServicePort != cfg.MultiDevice.ServicePort {
				return bkc.Config{}, fmt.Errorf("head service_port must be %d for BKC %s", cfg.MultiDevice.ServicePort, cfg.ID)
			}
			serviceAddress = serviceIP.String()
		} else if binding.ServiceAddress != "" || binding.ServicePort != 0 {
			return bkc.Config{}, fmt.Errorf("only the head binding may define a service endpoint")
		}
	}
	if _, overlapsFabric := addresses[serviceAddress]; overlapsFabric {
		return bkc.Config{}, fmt.Errorf("head service_address must be distinct from the private fabric addresses")
	}
	for _, role := range cfg.MultiDevice.Roles {
		if _, ok := roles[role.Name]; !ok {
			return bkc.Config{}, fmt.Errorf("missing exact role %q", role.Name)
		}
	}
	return cfg, nil
}

func validateAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("api_key is required")
	}
	if len(apiKey) > 512 || strings.IndexFunc(apiKey, unicode.IsSpace) >= 0 || strings.IndexFunc(apiKey, unicode.IsControl) >= 0 {
		return fmt.Errorf("api_key must be a single non-control token of at most 512 characters")
	}
	return nil
}

func normalizeRequest(request CreateRequest) CreateRequest {
	request.BKCID = strings.TrimSpace(request.BKCID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.LocalModelPath = strings.TrimSpace(request.LocalModelPath)
	for index := range request.Bindings {
		request.Bindings[index].Role = strings.TrimSpace(request.Bindings[index].Role)
		request.Bindings[index].DeviceID = strings.TrimSpace(request.Bindings[index].DeviceID)
		request.Bindings[index].FabricAddress = strings.TrimSpace(request.Bindings[index].FabricAddress)
		request.Bindings[index].ServiceAddress = strings.TrimSpace(request.Bindings[index].ServiceAddress)
		request.Bindings[index].ObservedContainerID = strings.TrimSpace(request.Bindings[index].ObservedContainerID)
	}
	return request
}

func requestHash(request CreateRequest) (string, error) {
	canonical := struct {
		BKCID          string    `json:"bkc_id"`
		IdempotencyKey string    `json:"idempotency_key"`
		Bindings       []Binding `json:"bindings"`
		LocalModelPath string    `json:"local_model_path,omitempty"`
	}{request.BKCID, request.IdempotencyKey, append([]Binding(nil), request.Bindings...), request.LocalModelPath}
	sort.Slice(canonical.Bindings, func(i, j int) bool { return canonical.Bindings[i].Role < canonical.Bindings[j].Role })
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("hash deployment request: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func orderedBindings(bindings []Binding, cfg bkc.Config) []Binding {
	byRole := make(map[string]Binding, len(bindings))
	for _, binding := range bindings {
		byRole[binding.Role] = binding
	}
	ordered := make([]Binding, 0, len(cfg.MultiDevice.Roles))
	for _, role := range cfg.MultiDevice.Roles {
		ordered = append(ordered, byRole[role.Name])
	}
	return ordered
}

func roleRank(cfg bkc.Config, roleName string) int {
	for _, role := range cfg.MultiDevice.Roles {
		if role.Name == roleName {
			return role.Rank
		}
	}
	return -1
}

func buildCandidate(deployment Deployment, request CreateRequest, cfg bkc.Config, binding Binding, headAddress string) Candidate {
	rank := roleRank(cfg, binding.Role)
	headForArg := headAddress
	if strings.Contains(headAddress, ":") {
		headForArg = "[" + headAddress + "]"
	}
	args := strings.ReplaceAll(cfg.ExtraArgs, bkc.MultiDeviceRoleRankPlaceholder, strconv.Itoa(rank))
	args = strings.ReplaceAll(args, bkc.MultiDeviceHeadAddrPlaceholder, headForArg)
	bindAddress := binding.FabricAddress
	if binding.Role == bkc.MultiDeviceRoleHead {
		bindAddress = binding.ServiceAddress
	}
	args = strings.TrimSpace(fmt.Sprintf("%s --host %s --port %d", args, bindAddress, cfg.MultiDevice.ServicePort))
	model := cfg.ModelID
	volumes := cloneMap(cfg.Volumes)
	if request.LocalModelPath != "" {
		model = FixedLocalModelPath
		volumes[request.LocalModelPath] = FixedLocalModelPath + ":ro"
	}
	labels := map[string]string{
		"io.yokai.managed": "true", "io.yokai.ownership": string(OwnershipManaged),
		"io.yokai.deployment.id": deployment.ID, "io.yokai.deployment.generation": strconv.Itoa(deployment.Generation),
		"io.yokai.deployment.role": binding.Role, "io.yokai.bkc.id": cfg.ID,
		"io.yokai.model.revision": cfg.MultiDevice.ModelRevision,
		"io.yokai.image.digest":   strings.TrimPrefix(cfg.Image[strings.Index(cfg.Image, "@")+1:], "sha256:"),
	}
	if len(cfg.MultiDevice.RuntimePatches) != 0 {
		labels["io.yokai.runtime.patch"] = bkc.GLM53FlashRuntimePatchSetLabel()
	}
	if binding.Role == bkc.MultiDeviceRoleHead {
		labels["io.yokai.service.address"] = binding.ServiceAddress
		labels["io.yokai.service.port"] = cfg.Port
	}
	runtime := cfg.Runtime
	runtime.ShmSize = "32g"
	runtime.Ulimits = map[string]string{"memlock": "-1", "stack": "67108864"}
	runtime.RestartPolicy = config.RestartPolicyNo
	var structuredArgs []string
	if binding.Role == bkc.MultiDeviceRoleHead {
		structuredArgs = []string{"--api-key=" + request.APIKey}
	}
	return Candidate{
		Role: binding.Role, Rank: rank, Name: fmt.Sprintf("yokai-deployment-%s-g%d-%s", deployment.ID, deployment.Generation, binding.Role),
		Image: cfg.Image, Model: model, Port: cfg.Port, ExtraArgs: args, Args: structuredArgs,
		Env: cloneMap(cfg.Env), Volumes: volumes, Runtime: runtime, Labels: labels,
		NetworkMode: "host", Devices: []string{"/dev/infiniband:/dev/infiniband"}, CapAdd: []string{"IPC_LOCK"}, GPUIDs: "0",
	}
}

func runtimePatchProvenance(cfg bkc.Config) []RuntimePatchProvenance {
	if cfg.MultiDevice == nil || len(cfg.MultiDevice.RuntimePatches) == 0 {
		return nil
	}
	provenance := make([]RuntimePatchProvenance, len(cfg.MultiDevice.RuntimePatches))
	for index, patch := range cfg.MultiDevice.RuntimePatches {
		provenance[index] = RuntimePatchProvenance{
			Label: patch.Label, SourcePath: patch.SourcePath,
			OriginalSHA256: patch.OriginalSHA256, PatchedSHA256: patch.PatchedSHA256,
		}
	}
	return provenance
}

func plannedMember(binding Binding, candidate Candidate, generation int) Member {
	return Member{Role: binding.Role, Rank: candidate.Rank, DeviceID: binding.DeviceID, FabricAddress: binding.FabricAddress, ServiceAddress: binding.ServiceAddress, ServicePort: binding.ServicePort, ContainerID: candidate.Name, Name: candidate.Name, Status: "planned", Ownership: OwnershipManaged, Generation: generation}
}

func memberFromObserved(binding Binding, rank int, observed ObservedContainer) Member {
	return Member{Role: binding.Role, Rank: rank, DeviceID: binding.DeviceID, FabricAddress: binding.FabricAddress, ServiceAddress: binding.ServiceAddress, ServicePort: binding.ServicePort, ContainerID: observed.ID, Name: observed.Name, Status: observed.Status, Ownership: observed.Ownership, Generation: observed.Generation}
}

func memberByRole(members []Member, role string) (Member, bool) {
	index := memberIndexByRole(members, role)
	if index < 0 {
		return Member{}, false
	}
	return members[index], true
}

func memberIndexByRole(members []Member, role string) int {
	for index, member := range members {
		if member.Role == role {
			return index
		}
	}
	return -1
}

func memberLocator(member Member) string {
	if member.ContainerID != "" {
		return member.ContainerID
	}
	return member.Name
}

func candidateMayExist(status string) bool {
	switch status {
	case "planned", "pulling", "pulled", "removed":
		return false
	default:
		return true
	}
}

func cloneBindings(src []Binding) []Binding { return append([]Binding(nil), src...) }

func cloneMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func randomDeploymentID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("dep-%d", time.Now().UnixNano())
	}
	return "dep-" + hex.EncodeToString(raw[:])
}
