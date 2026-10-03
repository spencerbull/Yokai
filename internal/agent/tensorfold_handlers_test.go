package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestTensorFoldResourceAPIRequiresAuthAndExactDurableIdentity(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), &fakeTensorFoldRunner{})
	mux := http.NewServeMux()
	registerTensorFoldRoutes(mux, manager)
	previousToken := authToken
	authToken = "test-agent-token"
	t.Cleanup(func() { authToken = previousToken })

	resourceRequest := validTensorFoldResourceRequest(false)
	body, err := json.Marshal(resourceRequest)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/deployments/tensorfold/resources", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated resource creation returned %d", unauthorized.Code)
	}

	create := httptest.NewRequest(http.MethodPost, "/deployments/tensorfold/resources", bytes.NewReader(body))
	create.Header.Set("Authorization", "Bearer test-agent-token")
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("authenticated resource creation returned %d: %s", created.Code, created.Body.String())
	}
	var resource tensorFoldResource
	if err := json.Unmarshal(created.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	if resource.ID != tensorFoldResourceID(resourceRequest.Name) || resource.DeploymentID != resourceRequest.DeploymentID || resource.Generation != resourceRequest.Generation || resource.Role != resourceRequest.Role {
		t.Fatalf("resource identity mismatch: %#v", resource)
	}

	forged := httptest.NewRequest(http.MethodGet, "/deployments/tensorfold/resources/"+resourceRequest.Name+"?deployment_id="+resourceRequest.DeploymentID+"&generation="+strconv.Itoa(resourceRequest.Generation+1)+"&role="+resourceRequest.Role, nil)
	forged.Header.Set("Authorization", "Bearer test-agent-token")
	forgedResponse := httptest.NewRecorder()
	mux.ServeHTTP(forgedResponse, forged)
	if forgedResponse.Code != http.StatusConflict {
		t.Fatalf("forged resource identity returned %d: %s", forgedResponse.Code, forgedResponse.Body.String())
	}

	exact := httptest.NewRequest(http.MethodGet, "/deployments/tensorfold/resources/"+resourceRequest.Name+"?deployment_id="+resourceRequest.DeploymentID+"&generation="+strconv.Itoa(resourceRequest.Generation)+"&role="+resourceRequest.Role, nil)
	exact.Header.Set("Authorization", "Bearer test-agent-token")
	exactResponse := httptest.NewRecorder()
	mux.ServeHTTP(exactResponse, exact)
	if exactResponse.Code != http.StatusOK {
		t.Fatalf("exact resource identity returned %d: %s", exactResponse.Code, exactResponse.Body.String())
	}
}
