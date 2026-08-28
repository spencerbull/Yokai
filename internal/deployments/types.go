package deployments

import (
	"context"
	"encoding/json"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

const (
	StoreVersion          = 1
	StoreFile             = "deployments.json"
	FixedLocalModelPath   = "/models/yokai-deployment"
	DefaultOwnershipLabel = "managed"
)

const (
	DefaultReadinessTimeout         = 45 * time.Minute
	DefaultReadinessInterval        = 5 * time.Second
	DefaultCleanupTimeout           = 5 * time.Minute
	DefaultImagePullRPCTimeout      = 10 * time.Minute
	DefaultMemberMutationRPCTimeout = 2 * time.Minute
	DefaultClientRequestTimeout     = 90 * time.Minute
	// A coordinated launch RPC outlives both the bounded docker CLI and its
	// post-exit deterministic-name settling pass. This makes the agent response
	// a cleanup barrier instead of allowing request cancellation to race rollback.
	DefaultCandidateLaunchCommandTimeout = 5 * time.Minute
	DefaultCandidateLaunchSettleTimeout  = 5 * time.Minute
	DefaultCandidateLaunchRPCTimeout     = 11 * time.Minute
	DefaultLogCaptureRPCTimeout          = 15 * time.Second
	MaxLogTailLines                      = 2000
	MaxLogTailBytes                      = 256 << 10
)

type State string

const (
	StatePending        State = "pending"
	StateStarting       State = "starting"
	StateRunning        State = "running"
	StateStopping       State = "stopping"
	StateStopped        State = "stopped"
	StateFailed         State = "failed"
	StateRolledBack     State = "rolled_back"
	StateRollbackFailed State = "rollback_failed"
)

type Phase string

const (
	PhasePlanned   Phase = "planned"
	PhasePulling   Phase = "pulling"
	PhaseCutover   Phase = "cutover"
	PhaseLaunching Phase = "launching"
	PhaseReadiness Phase = "readiness"
	PhaseRunning   Phase = "running"
	PhaseStopping  Phase = "stopping"
	PhaseStarting  Phase = "starting"
	PhaseRollback  Phase = "rollback"
	PhaseComplete  Phase = "complete"
)

type Ownership string

const (
	OwnershipManaged  Ownership = "managed"
	OwnershipAdopted  Ownership = "adopted"
	OwnershipObserved Ownership = "observed"
)

type Binding struct {
	Role                string `json:"role"`
	DeviceID            string `json:"device_id"`
	FabricAddress       string `json:"fabric_address"`
	ServiceAddress      string `json:"service_address,omitempty"`
	ServicePort         int    `json:"service_port,omitempty"`
	ObservedContainerID string `json:"observed_container_id,omitempty"`
}

// CreateRequest contains request-time inputs. APIKey and LocalModelPath are
// never copied into Deployment, the durable store, or API responses.
type CreateRequest struct {
	BKCID          string    `json:"bkc_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Bindings       []Binding `json:"bindings"`
	APIKey         string    `json:"api_key,omitempty"`
	LocalModelPath string    `json:"local_model_path,omitempty"`
}

// TestRequest carries a required request-time API key for an authenticated
// rank-0 probe. It is never copied into Deployment or the store.
type TestRequest struct {
	APIKey string `json:"api_key,omitempty"`
}

type Member struct {
	Role           string    `json:"role"`
	Rank           int       `json:"rank"`
	DeviceID       string    `json:"device_id"`
	FabricAddress  string    `json:"fabric_address"`
	ServiceAddress string    `json:"service_address,omitempty"`
	ServicePort    int       `json:"service_port,omitempty"`
	ContainerID    string    `json:"container_id"`
	Name           string    `json:"name,omitempty"`
	Status         string    `json:"status"`
	Ownership      Ownership `json:"ownership"`
	Generation     int       `json:"generation"`
}

type RollbackResult struct {
	Attempted           bool          `json:"attempted"`
	RemovedCandidates   []string      `json:"removed_candidates,omitempty"`
	RestartedContainers []string      `json:"restarted_containers,omitempty"`
	LogTails            []RankLogTail `json:"log_tails,omitempty"`
	Errors              []string      `json:"errors,omitempty"`
	CompletedAt         time.Time     `json:"completed_at"`
}

// RankLogTail is a bounded, sanitized diagnostic captured before rollback
// removes a managed candidate. Status distinguishes a genuinely empty log from
// a failed capture.
type RankLogTail struct {
	Role        string    `json:"role"`
	Rank        int       `json:"rank"`
	DeviceID    string    `json:"device_id"`
	ContainerID string    `json:"container_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	Status      string    `json:"status"`
	Tail        string    `json:"tail,omitempty"`
	Truncated   bool      `json:"truncated,omitempty"`
	Error       string    `json:"error,omitempty"`
	CapturedAt  time.Time `json:"captured_at"`
}

type LogTailCapture struct {
	Tail      string
	Truncated bool
}

type RuntimePatchProvenance struct {
	Label          string `json:"label"`
	SourcePath     string `json:"source_path"`
	OriginalSHA256 string `json:"original_sha256"`
	PatchedSHA256  string `json:"patched_sha256"`
}

type TestResult struct {
	OK           bool      `json:"ok"`
	MetricsReady bool      `json:"metrics_ready"`
	Message      string    `json:"message"`
	Model        string    `json:"model,omitempty"`
	Response     string    `json:"response,omitempty"`
	TestedAt     time.Time `json:"tested_at"`
}

// Progress records durable intent and completion around each external effect.
// Detail must remain free of request credentials, argv, environment, and host
// model paths.
type Progress struct {
	Phase  Phase     `json:"phase"`
	Action string    `json:"action"`
	Role   string    `json:"role,omitempty"`
	Status string    `json:"status"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

// Deployment is the complete safe durable/API representation. It contains no
// API key, environment, command line, or host-local model path.
type Deployment struct {
	ID                     string                   `json:"id"`
	BKCID                  string                   `json:"bkc_id"`
	IdempotencyKey         string                   `json:"idempotency_key"`
	RequestHash            string                   `json:"request_hash,omitempty"`
	State                  State                    `json:"state"`
	Phase                  Phase                    `json:"phase"`
	Generation             int                      `json:"generation"`
	PreviousGeneration     int                      `json:"previous_generation"`
	Bindings               []Binding                `json:"bindings"`
	Members                []Member                 `json:"members,omitempty"`
	PreviousMembers        []Member                 `json:"previous_members,omitempty"`
	UsesLocalModelSnapshot bool                     `json:"uses_local_model_snapshot,omitempty"`
	RuntimePatches         []RuntimePatchProvenance `json:"runtime_patches,omitempty"`
	Error                  string                   `json:"error,omitempty"`
	Rollback               *RollbackResult          `json:"rollback,omitempty"`
	LastTest               *TestResult              `json:"last_test,omitempty"`
	Progress               []Progress               `json:"progress,omitempty"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
}

// UnmarshalJSON keeps already-persisted singular runtime_patch provenance
// readable without claiming that the historical deployment carried the newer
// ordered patch set. The next ordinary store write uses runtime_patches with
// the one exact legacy entry.
func (d *Deployment) UnmarshalJSON(data []byte) error {
	type deploymentAlias Deployment
	var decoded deploymentAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var legacy struct {
		RuntimePatch *RuntimePatchProvenance `json:"runtime_patch"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*d = Deployment(decoded)
	if len(d.RuntimePatches) == 0 && legacy.RuntimePatch != nil {
		d.RuntimePatches = []RuntimePatchProvenance{*legacy.RuntimePatch}
	}
	return nil
}

type Candidate struct {
	Role        string
	Rank        int
	Name        string
	Image       string
	Model       string
	Port        string
	ExtraArgs   string
	Args        []string
	Env         map[string]string
	Volumes     map[string]string
	Runtime     config.RuntimeOptions
	Labels      map[string]string
	NetworkMode string
	Devices     []string
	CapAdd      []string
	GPUIDs      string
}

type PreflightRequest struct {
	Binding        Binding
	CandidateName  string
	LocalModelPath string
	ServicePort    int
	RendezvousPort int
	Head           bool
}

type ObservedContainer struct {
	ID           string
	Name         string
	Status       string
	Ownership    Ownership
	Managed      bool
	Generation   int
	DeploymentID string
	Role         string
}

// Operations is the narrow device mutation boundary. Preflight and Inspect
// must be read-only; every subsequent call is journaled by Engine first.
type Operations interface {
	Preflight(context.Context, PreflightRequest, []string, []string) error
	Pull(context.Context, Binding, string) error
	Inspect(context.Context, string, string) (ObservedContainer, error)
	Stop(context.Context, string, string) error
	StopManaged(context.Context, Deployment, Member) error
	Launch(context.Context, Binding, Candidate) (ObservedContainer, error)
	WaitRunning(context.Context, string, string) error
	Test(context.Context, string, string, string) (TestResult, error)
	CaptureManagedLogs(context.Context, Deployment, Member, string) (LogTailCapture, error)
	Remove(context.Context, string, string) error
	RemoveManaged(context.Context, Deployment, Member) error
	Restart(context.Context, string, string) error
	RestartManaged(context.Context, Deployment, Member) error
}
