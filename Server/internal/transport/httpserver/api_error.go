package httpserver

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type apiErrorCategory string

const (
	apiErrorAuthentication apiErrorCategory = "authentication"
	apiErrorAuthorization  apiErrorCategory = "authorization"
	apiErrorValidation     apiErrorCategory = "validation"
	apiErrorNotFound       apiErrorCategory = "not_found"
	apiErrorConflict       apiErrorCategory = "conflict"
	apiErrorRateLimit      apiErrorCategory = "rate_limit"
	apiErrorDependency     apiErrorCategory = "dependency"
	apiErrorInternal       apiErrorCategory = "internal"
)

type apiErrorDefinition struct {
	Code      string
	Category  apiErrorCategory
	Status    int
	Message   string
	Retryable bool
}

var apiErrorCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var sharedAPIErrorDefinitions = []apiErrorDefinition{
	{Code: "INVALID_REQUEST", Category: apiErrorValidation, Status: http.StatusBadRequest, Message: "Request is invalid"},
}

func respondError(c *gin.Context, status int, code, message string) {
	definition, ok := registeredAPIErrorDefinition(status, code)
	if !ok {
		definition = legacyAPIErrorDefinition(status, code, message)
	} else {
		definition.Message = message
	}
	respondDefinedError(c, definition)
}

func respondDefinedError(c *gin.Context, definition apiErrorDefinition) {
	c.JSON(definition.Status, contract.ErrorResponse{
		Code: definition.Code, Category: contract.ErrorResponseCategory(definition.Category), Message: definition.Message,
		RequestId: c.GetHeader(requestIDHeader), Retryable: definition.Retryable,
	})
}

func legacyAPIErrorDefinition(status int, code, message string) apiErrorDefinition {
	category := apiErrorCategoryForStatus(status)
	return apiErrorDefinition{
		Code: code, Category: category, Status: status, Message: message,
		Retryable: category == apiErrorDependency || category == apiErrorRateLimit,
	}
}

func registeredAPIErrorDefinition(status int, code string) (apiErrorDefinition, bool) {
	for _, definitions := range apiErrorDefinitionSets() {
		for _, definition := range definitions {
			if definition.Status == status && definition.Code == code {
				return definition, true
			}
		}
	}
	return apiErrorDefinition{}, false
}

func apiErrorDefinitionSets() [][]apiErrorDefinition {
	return [][]apiErrorDefinition{
		sharedAPIErrorDefinitions,
		authAPIErrorDefinitions,
		accessAPIErrorDefinitions,
		localAuthAPIErrorDefinitions,
		transportAPIErrorDefinitions,
		workflowAPIErrorDefinitions,
		auditAPIErrorDefinitions,
	}
}

func apiErrorCategoryForStatus(status int) apiErrorCategory {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return apiErrorValidation
	case http.StatusUnauthorized:
		return apiErrorAuthentication
	case http.StatusForbidden:
		return apiErrorAuthorization
	case http.StatusNotFound:
		return apiErrorNotFound
	case http.StatusConflict:
		return apiErrorConflict
	case http.StatusTooManyRequests:
		return apiErrorRateLimit
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return apiErrorDependency
	default:
		return apiErrorInternal
	}
}

func validateAPIErrorDefinitions(definitions []apiErrorDefinition) error {
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if _, exists := seen[definition.Code]; exists {
			return fmt.Errorf("API error code %q is duplicated", definition.Code)
		}
		seen[definition.Code] = struct{}{}
		if err := validateAPIErrorDefinition(definition); err != nil {
			return err
		}
	}
	return nil
}

func validateAPIErrorDefinition(definition apiErrorDefinition) error {
	if !apiErrorCodePattern.MatchString(definition.Code) {
		return fmt.Errorf("API error code %q is invalid", definition.Code)
	}
	if definition.Category != apiErrorCategoryForStatus(definition.Status) {
		return fmt.Errorf("API error code %q category does not match status", definition.Code)
	}
	wantRetryable := definition.Category == apiErrorDependency || definition.Category == apiErrorRateLimit
	if definition.Retryable != wantRetryable {
		return fmt.Errorf("API error code %q retry semantics are invalid", definition.Code)
	}
	if definition.Message == "" {
		return fmt.Errorf("API error code %q message is empty", definition.Code)
	}
	return nil
}
