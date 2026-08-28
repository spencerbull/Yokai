package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/deployments"
)

// RunDeployments dispatches coordinated deployment subcommands.
func RunDeployments(args []string) {
	if len(args) == 0 {
		exitError("usage: yokai deployments <create|list|status|test|start|stop|rollback>")
	}
	switch args[0] {
	case "create":
		runDeploymentsCreate(args[1:])
	case "list":
		runDeploymentsList(args[1:])
	case "status":
		runDeploymentGet("status", args[1:])
	case "test":
		runDeploymentTest(args[1:])
	case "stop":
		runDeploymentAction("stop", args[1:])
	case "start":
		runDeploymentKeyAction("start", args[1:])
	case "rollback":
		runDeploymentAction("rollback", args[1:])
	default:
		exitError(fmt.Sprintf("unknown deployments subcommand: %s", args[0]))
	}
}

type deploymentCreateFlags struct {
	bkcID, key                   string
	headDevice, headFabric       string
	headServiceAddress           string
	headServicePort              string
	workerDevice, workerFabric   string
	headObserved, workerObserved string
	apiKeyEnv, localModelPath    string
}

func runDeploymentsCreate(args []string) {
	fs := flag.NewFlagSet("deployments create", flag.ExitOnError)
	var values deploymentCreateFlags
	fs.StringVar(&values.bkcID, "bkc", "", "multi-device BKC id")
	fs.StringVar(&values.key, "idempotency-key", "", "stable idempotency key")
	fs.StringVar(&values.headDevice, "head-device", "", "rank-0 device id")
	fs.StringVar(&values.headFabric, "head-fabric", "", "rank-0 fabric IP")
	fs.StringVar(&values.headServiceAddress, "head-service-address", "", "rank-0 client/monitor IP")
	fs.StringVar(&values.headServicePort, "head-service-port", "", "rank-0 client/monitor port (8000 for GLM-5.3)")
	fs.StringVar(&values.workerDevice, "worker-device", "", "rank-1 device id")
	fs.StringVar(&values.workerFabric, "worker-fabric", "", "rank-1 fabric IP")
	fs.StringVar(&values.headObserved, "head-observed-container", "", "explicit old rank-0 container id to stop/restart")
	fs.StringVar(&values.workerObserved, "worker-observed-container", "", "explicit old rank-1 container id to stop/restart")
	fs.StringVar(&values.apiKeyEnv, "api-key-env", "", "environment variable containing the request-time API key")
	fs.StringVar(&values.localModelPath, "local-model-path", "", "absolute snapshot path present on both devices")
	_ = fs.Parse(args)
	request, err := buildDeploymentCreateRequest(values, os.Getenv)
	if err != nil {
		exitError(err.Error())
	}
	body, err := json.Marshal(request)
	if err != nil {
		exitError(fmt.Sprintf("encoding deployment request: %v", err))
	}
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}
	data, err := newDeploymentDaemonClient(cfg).post("/deployments", bytes.NewReader(body))
	if err != nil {
		exitError(err.Error())
	}
	outputRaw(data)
}

func buildDeploymentCreateRequest(values deploymentCreateFlags, getenv func(string) string) (deployments.CreateRequest, error) {
	required := map[string]string{
		"--bkc": values.bkcID, "--idempotency-key": values.key,
		"--head-device": values.headDevice, "--head-fabric": values.headFabric,
		"--head-service-address": values.headServiceAddress, "--head-service-port": values.headServicePort,
		"--worker-device": values.workerDevice, "--worker-fabric": values.workerFabric,
		"--api-key-env": values.apiKeyEnv,
	}
	for flagName, value := range required {
		if strings.TrimSpace(value) == "" {
			return deployments.CreateRequest{}, fmt.Errorf("%s is required", flagName)
		}
	}
	if values.headDevice == values.workerDevice {
		return deployments.CreateRequest{}, fmt.Errorf("head and worker devices must be distinct")
	}
	if net.ParseIP(values.headFabric) == nil || net.ParseIP(values.workerFabric) == nil {
		return deployments.CreateRequest{}, fmt.Errorf("head and worker fabric addresses must be valid IPs")
	}
	serviceIP := net.ParseIP(values.headServiceAddress)
	if serviceIP == nil || serviceIP.IsUnspecified() || serviceIP.IsLoopback() || serviceIP.IsMulticast() {
		return deployments.CreateRequest{}, fmt.Errorf("head service address must be an explicit valid IP")
	}
	if serviceIP.Equal(net.ParseIP(values.headFabric)) || serviceIP.Equal(net.ParseIP(values.workerFabric)) {
		return deployments.CreateRequest{}, fmt.Errorf("head service address must be distinct from fabric addresses")
	}
	servicePort, err := strconv.Atoi(values.headServicePort)
	if err != nil || servicePort < 1 || servicePort > 65535 {
		return deployments.CreateRequest{}, fmt.Errorf("head service port must be between 1 and 65535")
	}
	apiKey := getenv(values.apiKeyEnv)
	if apiKey == "" {
		return deployments.CreateRequest{}, fmt.Errorf("environment variable %s is empty", values.apiKeyEnv)
	}
	return deployments.CreateRequest{
		BKCID: values.bkcID, IdempotencyKey: values.key, APIKey: apiKey, LocalModelPath: values.localModelPath,
		Bindings: []deployments.Binding{
			{Role: "head", DeviceID: values.headDevice, FabricAddress: values.headFabric, ServiceAddress: values.headServiceAddress, ServicePort: servicePort, ObservedContainerID: values.headObserved},
			{Role: "worker", DeviceID: values.workerDevice, FabricAddress: values.workerFabric, ObservedContainerID: values.workerObserved},
		},
	}, nil
}

func runDeploymentsList(args []string) {
	fs := flag.NewFlagSet("deployments list", flag.ExitOnError)
	_ = fs.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}
	data, err := newDeploymentDaemonClient(cfg).get("/deployments")
	if err != nil {
		exitError(err.Error())
	}
	outputRaw(data)
}

func runDeploymentGet(action string, args []string) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		exitError(fmt.Sprintf("usage: yokai deployments %s <deployment-id>", action))
	}
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}
	data, err := newDeploymentDaemonClient(cfg).get("/deployments/" + url.PathEscape(args[0]))
	if err != nil {
		exitError(err.Error())
	}
	outputRaw(data)
}

func runDeploymentAction(action string, args []string) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		exitError(fmt.Sprintf("usage: yokai deployments %s <deployment-id>", action))
	}
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}
	path := fmt.Sprintf("/deployments/%s/%s", url.PathEscape(args[0]), action)
	data, err := newDeploymentDaemonClient(cfg).post(path, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		exitError(err.Error())
	}
	outputRaw(data)
}

func runDeploymentTest(args []string) {
	runDeploymentKeyAction("test", args)
}

func runDeploymentKeyAction(action string, args []string) {
	fs := flag.NewFlagSet("deployments "+action, flag.ExitOnError)
	keyHelp := "environment variable containing the request-time API key"
	if action == "start" {
		keyHelp = "environment variable containing the original rank-0 launch API key (start cannot rotate it)"
	}
	apiKeyEnv := fs.String("api-key-env", "", keyHelp)
	_ = fs.Parse(args)
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		exitError(fmt.Sprintf("usage: yokai deployments %s --api-key-env NAME <deployment-id>", action))
	}
	request, err := buildDeploymentTestRequest(*apiKeyEnv, os.Getenv)
	if err != nil {
		exitError(err.Error())
	}
	body, err := json.Marshal(request)
	if err != nil {
		exitError(fmt.Sprintf("encoding deployment %s request: %v", action, err))
	}
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}
	path := fmt.Sprintf("/deployments/%s/%s", url.PathEscape(fs.Arg(0)), action)
	data, err := newDeploymentDaemonClient(cfg).post(path, bytes.NewReader(body))
	if err != nil {
		exitError(err.Error())
	}
	outputRaw(data)
}

func buildDeploymentTestRequest(apiKeyEnv string, getenv func(string) string) (deployments.TestRequest, error) {
	if strings.TrimSpace(apiKeyEnv) == "" {
		return deployments.TestRequest{}, fmt.Errorf("--api-key-env is required")
	}
	apiKey := getenv(apiKeyEnv)
	if apiKey == "" {
		return deployments.TestRequest{}, fmt.Errorf("environment variable %s is empty", apiKeyEnv)
	}
	return deployments.TestRequest{APIKey: apiKey}, nil
}

func newDeploymentDaemonClient(cfg *config.Config) *daemonClient {
	client := newDaemonClient(cfg)
	// Two staged pulls plus the 45-minute readiness window can legitimately
	// exceed the generic command timeout. Keep this finite but patient enough
	// that the CLI does not cancel a healthy 181 GiB canary.
	client.http.Timeout = deployments.DefaultClientRequestTimeout
	return client
}
