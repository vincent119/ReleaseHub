package httpserver

import "net/http"

var authAPIErrorDefinitions = []apiErrorDefinition{
	{Code: "AUTH_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Authentication is unavailable", Retryable: true},
	{Code: "AUTH_LOGIN_FAILED", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Unable to start authentication", Retryable: true},
	{Code: "AUTH_STATE_MISSING", Category: apiErrorValidation, Status: http.StatusBadRequest, Message: "Authentication state is missing"},
	{Code: "AUTH_CALLBACK_FAILED", Category: apiErrorAuthentication, Status: http.StatusUnauthorized, Message: "Authentication callback was rejected"},
	{Code: "LOGOUT_TOKEN_REQUIRED", Category: apiErrorValidation, Status: http.StatusBadRequest, Message: "Logout token is required"},
	{Code: "LOGOUT_TOKEN_INVALID", Category: apiErrorValidation, Status: http.StatusBadRequest, Message: "Logout token was rejected"},
	{Code: "ORIGIN_REJECTED", Category: apiErrorAuthorization, Status: http.StatusForbidden, Message: "Request origin is not allowed"},
	{Code: "SESSION_REQUIRED", Category: apiErrorAuthentication, Status: http.StatusUnauthorized, Message: "Authentication is required"},
	{Code: "SESSION_INVALID", Category: apiErrorAuthentication, Status: http.StatusUnauthorized, Message: "Authentication session is invalid"},
	{Code: "MUTATION_REJECTED", Category: apiErrorAuthorization, Status: http.StatusForbidden, Message: "State-changing request was rejected"},
	{Code: "PASSWORD_CHANGE_REQUIRED", Category: apiErrorAuthorization, Status: http.StatusForbidden, Message: "The initial password must be changed"},
	{Code: "SESSION_LOGOUT_REJECTED", Category: apiErrorAuthorization, Status: http.StatusForbidden, Message: "Logout request was rejected"},
}

var localAuthAPIErrorDefinitions = []apiErrorDefinition{
	{Code: "LOCAL_AUTH_UNAVAILABLE", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Local authentication is unavailable"},
	{Code: "INVALID_CREDENTIALS", Category: apiErrorAuthentication, Status: http.StatusUnauthorized, Message: "Username or password is invalid"},
	{Code: "PASSWORD_CHANGE_REJECTED", Category: apiErrorValidation, Status: http.StatusBadRequest, Message: "Password change was rejected"},
}

var (
	accessManagementNotFoundError = apiErrorDefinition{Code: "ACCESS_MANAGEMENT_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Access management resource was not found"}
	accessSelfDisableError        = apiErrorDefinition{Code: "ACCESS_SELF_DISABLE_FORBIDDEN", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Administrators cannot disable their own account"}
	accessLastManagerError        = apiErrorDefinition{Code: "ACCESS_LAST_PLATFORM_MANAGER", Category: apiErrorConflict, Status: http.StatusConflict, Message: "The last platform-management path cannot be removed"}
	accessResourceProtectedError  = apiErrorDefinition{Code: "ACCESS_RESOURCE_PROTECTED", Category: apiErrorConflict, Status: http.StatusConflict, Message: "The access-management resource is system managed"}
	accessManagementConflictError = apiErrorDefinition{Code: "ACCESS_MANAGEMENT_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "The access-management resource is no longer active"}
	accessAPIErrorDefinitions     = []apiErrorDefinition{
		{Code: "LOCAL_USER_CREATION_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Local user creation is unavailable", Retryable: true},
		accessManagementNotFoundError,
		{Code: "ACCESS_USERNAME_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "The username is unavailable"},
		{Code: "ACCESS_USER_CREATE_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to create local user"},
		{Code: "ACCESS_MANAGEMENT_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Access management is unavailable", Retryable: true},
		{Code: "ACCESS_MANAGEMENT_READ_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read access management"},
		accessSelfDisableError,
		accessLastManagerError,
		accessResourceProtectedError,
		accessManagementConflictError,
		{Code: "ACCESS_MANAGEMENT_MUTATION_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to apply the access management change"},
	}
)

var (
	workflowUnavailableError = apiErrorDefinition{
		Code: "WORKFLOW_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable,
		Message: "Release workflow is unavailable", Retryable: true,
	}
	workflowNotFoundError = apiErrorDefinition{
		Code: "WORKFLOW_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound,
		Message: "Release workflow was not found",
	}
	workflowReadFailedError = apiErrorDefinition{
		Code: "WORKFLOW_READ_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError,
		Message: "Unable to read release workflows",
	}
	workflowMutationFailedError = apiErrorDefinition{
		Code: "WORKFLOW_MUTATION_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError,
		Message: "Unable to change release workflow",
	}
	workflowNameConflictError = apiErrorDefinition{
		Code: "WORKFLOW_NAME_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict,
		Message: "Release workflow name already exists",
	}
	workflowVersionConflictError = apiErrorDefinition{
		Code: "WORKFLOW_VERSION_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict,
		Message: "Release workflow version changed",
	}
	workflowDraftExistsError = apiErrorDefinition{
		Code: "WORKFLOW_DRAFT_EXISTS", Category: apiErrorConflict, Status: http.StatusConflict,
		Message: "Release workflow already has a draft",
	}
	workflowConflictError = apiErrorDefinition{
		Code: "WORKFLOW_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict,
		Message: "Release workflow change was rejected",
	}
	workflowAPIErrorDefinitions = []apiErrorDefinition{
		workflowUnavailableError, workflowNotFoundError, workflowReadFailedError,
		workflowMutationFailedError, workflowNameConflictError, workflowVersionConflictError,
		workflowDraftExistsError, workflowConflictError,
	}
)

var transportAPIErrorDefinitions = []apiErrorDefinition{
	{Code: "RESOURCE_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Resource was not found"},
	{Code: "RESOURCE_MUTATION_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Resource changed concurrently or conflicts with catalog data"},
	{Code: "RESOURCE_MUTATION_REJECTED", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Resource change was rejected"},
	{Code: "CATALOG_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Resource catalog is unavailable", Retryable: true},
	{Code: "CATALOG_READ_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read resource catalog"},
	{Code: "CATALOG_APPLICATION_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Catalog Application was not found"},
	{Code: "APPLICATION_STATUS_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Application status is unavailable", Retryable: true},
	{Code: "CANDIDATE_QUEUE_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Argo CD candidate queue is unavailable", Retryable: true},
	{Code: "ONBOARDING_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Application onboarding is unavailable", Retryable: true},
	{Code: "CANDIDATE_QUEUE_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Argo CD candidate queue was not found"},
	{Code: "CANDIDATE_ASSIGNMENT_SCOPES_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Candidate assignment scopes were not found"},
	{Code: "CANDIDATE_QUEUE_READ_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read Argo CD candidate queue"},
	{Code: "CANDIDATE_ASSIGNMENT_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Argo CD candidate assignment was rejected"},
	{Code: "ONBOARDING_CONFIRM_REJECTED", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Application onboarding confirmation was rejected"},
	{Code: "ONBOARDING_DRY_RUN_REJECTED", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Application onboarding validation failed"},
	{Code: "DEPLOYMENT_PLAN_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Deployment Plan is unavailable", Retryable: true},
	{Code: "DEPLOYMENT_BINDING_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Deployment binding is unavailable", Retryable: true},
	{Code: "DEPLOYMENT_PLAN_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Deployment Plan was not found"},
	{Code: "DEPLOYMENT_PLAN_READ_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read Deployment Plans"},
	{Code: "DEPLOYMENT_PLAN_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Deployment Plan change was rejected"},
	{Code: "DEPLOYMENT_PLAN_MUTATION_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to change Deployment Plan"},
	{Code: "DEPLOYMENT_BINDING_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Deployment binding scope was not found"},
	{Code: "DEPLOYMENT_BINDING_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Deployment binding change was rejected"},
	{Code: "DEPLOYMENT_BINDING_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read or change deployment binding"},
	{Code: "DEPLOYMENT_FEATURE_UNAVAILABLE", Category: apiErrorInternal, Status: http.StatusNotImplemented, Message: "Deployment feature is not available"},
	{Code: "DEPLOYMENT_EXECUTION_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Deployment execution was not found"},
	{Code: "DEPLOYMENT_EXECUTION_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Deployment execution changed concurrently"},
	{Code: "DEPLOYMENT_EXECUTION_INVALID", Category: apiErrorValidation, Status: http.StatusUnprocessableEntity, Message: "Deployment execution command is invalid"},
	{Code: "DEPLOYMENT_EXECUTION_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to process deployment execution"},
	{Code: "DEPLOYMENT_HISTORY_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Deployment history was not found"},
	{Code: "DEPLOYMENT_HISTORY_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read deployment history"},
	{Code: "DEPLOYMENT_REQUEST_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Deployment Request was not found"},
	{Code: "DEPLOYMENT_REQUEST_INVALID", Category: apiErrorValidation, Status: http.StatusUnprocessableEntity, Message: "Deployment Request is invalid"},
	{Code: "DEPLOYMENT_REQUEST_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to read Deployment Request"},
	{Code: "DEPLOYMENT_WORKFLOW_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Deployment workflow was not found"},
	{Code: "DEPLOYMENT_WORKFLOW_CONFLICT", Category: apiErrorConflict, Status: http.StatusConflict, Message: "Deployment workflow changed concurrently"},
	{Code: "DEPLOYMENT_WORKFLOW_INVALID", Category: apiErrorValidation, Status: http.StatusUnprocessableEntity, Message: "Deployment workflow transition is invalid"},
	{Code: "DEPLOYMENT_WORKFLOW_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to update deployment workflow"},
	{Code: "NOTIFICATION_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Notification was not found"},
	{Code: "NOTIFICATION_FAILED", Category: apiErrorInternal, Status: http.StatusInternalServerError, Message: "Unable to process notification"},
}
