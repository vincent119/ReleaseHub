package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

// GetDeploymentSchedule returns one effective Environment policy.
func (h *planHandler) GetDeploymentSchedule(c *gin.Context, environmentID uuid.UUID) {
	principal, ok := h.authenticateSchedule(c, "")
	if !ok {
		return
	}
	view, err := h.schedules.Get(c.Request.Context(), principal, environmentID)
	if !respondDeploymentScheduleError(c, err) {
		return
	}
	c.JSON(http.StatusOK, deploymentScheduleResponse(view, c))
}

// UpdateDeploymentSchedule replaces one Environment policy.
func (h *planHandler) UpdateDeploymentSchedule(c *gin.Context, environmentID uuid.UUID, params contract.UpdateDeploymentScheduleParams) {
	principal, ok := h.authenticateSchedule(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	input, err := deploymentScheduleInput(c, environmentID, string(params.IdempotencyKey))
	if err != nil {
		respondError(c, http.StatusBadRequest, "DEPLOYMENT_SCHEDULE_INVALID", "Deployment schedule is invalid")
		return
	}
	view, err := h.schedules.Put(c.Request.Context(), principal, input)
	if !respondDeploymentScheduleError(c, err) {
		return
	}
	c.JSON(http.StatusOK, deploymentScheduleResponse(view, c))
}

func (h *planHandler) authenticateSchedule(c *gin.Context, csrf string) (deployapp.PlanPrincipal, bool) {
	userPrincipal, ok := h.deploymentSchedulePrincipal(c, csrf)
	if !ok {
		return deployapp.PlanPrincipal{}, false
	}
	if h.schedules == nil {
		respondError(c, http.StatusServiceUnavailable, "DEPLOYMENT_SCHEDULE_UNAVAILABLE", "Deployment schedule is unavailable")
		return deployapp.PlanPrincipal{}, false
	}
	return userPrincipal, true
}

func (h *planHandler) deploymentSchedulePrincipal(c *gin.Context, csrf string) (deployapp.PlanPrincipal, bool) {
	if csrf == "" {
		_, user, ok := h.authn.authenticate(c)
		return deployapp.PlanPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
	}
	_, user, ok := h.authn.authenticateMutation(c, csrf)
	return deployapp.PlanPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
}

func deploymentScheduleInput(c *gin.Context, environmentID uuid.UUID, idempotencyKey string) (deployapp.UpdateDeploymentScheduleInput, error) {
	var body contract.UpdateDeploymentScheduleRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 0 {
		return deployapp.UpdateDeploymentScheduleInput{}, errors.New("deployment schedule request is invalid")
	}
	return deployapp.UpdateDeploymentScheduleInput{
		EnvironmentID: environmentID, Enabled: body.Enabled, TimeZone: body.TimeZone,
		WeeklyWindows: deploymentScheduleWindows(body.WeeklyWindows), Blackouts: deploymentScheduleBlackouts(body.Blackouts),
		ExpectedVersion: uint64(body.ExpectedVersion),
		IdempotencyKey:  idempotencyKey, RequestID: c.GetHeader(requestIDHeader),
	}, nil
}

func deploymentScheduleWindows(values []contract.DeploymentScheduleWeeklyWindow) []deploydomain.DeploymentScheduleWeeklyWindow {
	result := make([]deploydomain.DeploymentScheduleWeeklyWindow, 0, len(values))
	for _, value := range values {
		result = append(result, deploydomain.DeploymentScheduleWeeklyWindow{
			DayOfWeek: time.Weekday(value.DayOfWeek), StartMinute: value.StartMinute, EndMinute: value.EndMinute,
		})
	}
	return result
}

func deploymentScheduleBlackouts(values []contract.DeploymentScheduleBlackout) []deploydomain.DeploymentScheduleBlackout {
	result := make([]deploydomain.DeploymentScheduleBlackout, 0, len(values))
	for _, value := range values {
		result = append(result, deploydomain.DeploymentScheduleBlackout{StartsAt: value.StartsAt, EndsAt: value.EndsAt})
	}
	return result
}

func deploymentScheduleResponse(view deployapp.DeploymentScheduleView, c *gin.Context) contract.DeploymentScheduleResponse {
	windows := make([]contract.DeploymentScheduleWeeklyWindow, 0, len(view.Policy.WeeklyWindows))
	for _, window := range view.Policy.WeeklyWindows {
		windows = append(windows, contract.DeploymentScheduleWeeklyWindow{
			DayOfWeek: int(window.DayOfWeek), StartMinute: window.StartMinute, EndMinute: window.EndMinute,
		})
	}
	blackouts := make([]contract.DeploymentScheduleBlackout, 0, len(view.Policy.Blackouts))
	for _, blackout := range view.Policy.Blackouts {
		blackouts = append(blackouts, contract.DeploymentScheduleBlackout{StartsAt: blackout.StartsAt, EndsAt: blackout.EndsAt})
	}
	return contract.DeploymentScheduleResponse{
		Data: contract.DeploymentSchedule{
			EnvironmentId: view.Policy.EnvironmentID, Enabled: view.Policy.Enabled, TimeZone: view.Policy.TimeZone,
			WeeklyWindows: windows, Blackouts: blackouts, Version: int64(view.Policy.Version), CanManage: view.CanManage,
		},
		Meta: responseMeta(c),
	}
}

func respondDeploymentScheduleError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrDeploymentScheduleNotFound) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_SCHEDULE_NOT_FOUND", "Deployment schedule was not found")
		return false
	}
	if errors.Is(err, deployapp.ErrDeploymentScheduleInvalid) {
		respondError(c, http.StatusBadRequest, "DEPLOYMENT_SCHEDULE_INVALID", "Deployment schedule is invalid")
		return false
	}
	if errors.Is(err, deployapp.ErrDeploymentScheduleConflict) {
		respondError(c, http.StatusConflict, "DEPLOYMENT_SCHEDULE_CONFLICT", "Deployment schedule change was rejected")
		return false
	}
	respondError(c, http.StatusInternalServerError, "DEPLOYMENT_SCHEDULE_UNAVAILABLE", "Deployment schedule is unavailable")
	return false
}
