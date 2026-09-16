package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func TestAPIErrorCategoryAndRetrySemantics(t *testing.T) {
	tests := []struct {
		status    int
		category  apiErrorCategory
		retryable bool
	}{
		{status: http.StatusBadRequest, category: apiErrorValidation},
		{status: http.StatusUnprocessableEntity, category: apiErrorValidation},
		{status: http.StatusUnauthorized, category: apiErrorAuthentication},
		{status: http.StatusForbidden, category: apiErrorAuthorization},
		{status: http.StatusNotFound, category: apiErrorNotFound},
		{status: http.StatusConflict, category: apiErrorConflict},
		{status: http.StatusTooManyRequests, category: apiErrorRateLimit, retryable: true},
		{status: http.StatusBadGateway, category: apiErrorDependency, retryable: true},
		{status: http.StatusServiceUnavailable, category: apiErrorDependency, retryable: true},
		{status: http.StatusGatewayTimeout, category: apiErrorDependency, retryable: true},
		{status: http.StatusInternalServerError, category: apiErrorInternal},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			definition := legacyAPIErrorDefinition(test.status, "TEST_ERROR", "Safe message")
			if definition.Category != test.category || definition.Retryable != test.retryable {
				t.Fatalf("definition = %#v", definition)
			}
		})
	}
}

func TestRespondErrorWritesUnifiedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/release-workflows", nil)
	context.Request.Header.Set(requestIDHeader, "request-123")

	respondError(context, http.StatusServiceUnavailable, "WORKFLOW_UNAVAILABLE", "Release workflow is unavailable")

	var body contract.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusServiceUnavailable || body.Code != "WORKFLOW_UNAVAILABLE" || body.Category != contract.ErrorResponseCategory(apiErrorDependency) || !body.Retryable || body.RequestId != "request-123" {
		t.Fatalf("response = %d %#v", response.Code, body)
	}
}

func TestValidateAPIErrorDefinitionsRejectsInvalidRegistry(t *testing.T) {
	valid := apiErrorDefinition{
		Code: "WORKFLOW_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound,
		Message: "Release workflow was not found",
	}
	if err := validateAPIErrorDefinitions([]apiErrorDefinition{valid}); err != nil {
		t.Fatalf("valid definitions: %v", err)
	}

	tests := []struct {
		name        string
		definitions []apiErrorDefinition
	}{
		{name: "invalid code", definitions: []apiErrorDefinition{{Code: "invalid-code", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Safe"}}},
		{name: "duplicate code", definitions: []apiErrorDefinition{valid, valid}},
		{name: "category mismatch", definitions: []apiErrorDefinition{{Code: "WRONG_CATEGORY", Category: apiErrorConflict, Status: http.StatusNotFound, Message: "Safe"}}},
		{name: "retry mismatch", definitions: []apiErrorDefinition{{Code: "DEPENDENCY_ERROR", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Safe"}}},
		{name: "empty message", definitions: []apiErrorDefinition{{Code: "EMPTY_MESSAGE", Category: apiErrorInternal, Status: http.StatusInternalServerError}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAPIErrorDefinitions(test.definitions); err == nil {
				t.Fatal("expected registry validation error")
			}
		})
	}
}

func TestWorkflowAPIErrorDefinitionsFormAValidRegistry(t *testing.T) {
	if err := validateAPIErrorDefinitions(workflowAPIErrorDefinitions); err != nil {
		t.Fatalf("Workflow API error definitions: %v", err)
	}
}

func TestAuthenticationAPIErrorDefinitionsFormAValidRegistry(t *testing.T) {
	definitions := append([]apiErrorDefinition{}, sharedAPIErrorDefinitions...)
	definitions = append(definitions, authAPIErrorDefinitions...)
	definitions = append(definitions, localAuthAPIErrorDefinitions...)
	if err := validateAPIErrorDefinitions(definitions); err != nil {
		t.Fatalf("authentication API error definitions: %v", err)
	}
}

func TestAccessAPIErrorDefinitionsFormAValidRegistry(t *testing.T) {
	if err := validateAPIErrorDefinitions(accessAPIErrorDefinitions); err != nil {
		t.Fatalf("Access API error definitions: %v", err)
	}
}

func TestTransportAPIErrorDefinitionsFormAValidRegistry(t *testing.T) {
	if err := validateAPIErrorDefinitions(transportAPIErrorDefinitions); err != nil {
		t.Fatalf("transport API error definitions: %v", err)
	}
}

func TestAPIErrorRegistryHasUniqueDefinitionsAcrossFamilies(t *testing.T) {
	definitions := make([]apiErrorDefinition, 0)
	for _, family := range apiErrorDefinitionSets() {
		definitions = append(definitions, family...)
	}
	if err := validateAPIErrorDefinitions(definitions); err != nil {
		t.Fatalf("API error registry: %v", err)
	}
}
