package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spencerbull/yokai/internal/bkc"
)

func TestLegacyDeployRejectsMultiDeviceBKCBeforeMutation(t *testing.T) {
	d := &Daemon{}
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(`{"bkc_id":"`+bkc.GLM53FlashNVFP4DualGB10ID+`"}`))
	recorder := httptest.NewRecorder()
	d.handleDeploy(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "multi_device_required") {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestLegacyDeployRejectsMultiDeviceRecipeWhenBKCIDOmitted(t *testing.T) {
	d := &Daemon{}
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(`{"service_type":"sglang","model":"`+bkc.GLM53FlashNVFP4Model+`","image":"`+bkc.GLM53FlashNVFP4Image+`"}`))
	recorder := httptest.NewRecorder()
	d.handleDeploy(recorder, req)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "multi_device_required") {
		t.Fatalf("omitted bkc_id bypassed cluster guard: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestLegacyDeployRejectsMultiDevicePayloadWithUnrelatedSingleDeviceBKC(t *testing.T) {
	d := &Daemon{}
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(`{"bkc_id":"glm-4-5-air-fp8","service_type":"sglang","model":"`+bkc.GLM53FlashNVFP4Model+`","image":"`+bkc.GLM53FlashNVFP4Image+`"}`))
	recorder := httptest.NewRecorder()
	d.handleDeploy(recorder, req)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "multi_device_required") {
		t.Fatalf("unrelated bkc_id bypassed cluster payload guard: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeployBKCRecordIncludesTypedMultiDeviceMetadata(t *testing.T) {
	cfg, _ := bkc.LookupID(bkc.GLM53FlashNVFP4DualGB10ID)
	record := deployBKCRecordFromConfig(cfg, bkc.MatchExact, "")
	if record.MultiDevice == nil || record.MultiDevice.Roles[0].Name != bkc.MultiDeviceRoleHead {
		t.Fatalf("missing cluster metadata: %#v", record.MultiDevice)
	}
}
