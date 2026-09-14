package httpserver

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func accessSnapshotResponse(value authzapp.AccessSnapshot) contract.AccessManagementSnapshot {
	return contract.AccessManagementSnapshot{
		CanManagePlatform: value.CanManagePlatform, ManagedProjectIds: value.ManagedProjects,
		Users: accessUsersResponse(value.Users), Groups: accessGroupsResponse(value.Groups),
		Roles: accessRolesResponse(value.Roles), Permissions: accessPermissionsResponse(value.Permissions),
		Memberships: accessMembershipsResponse(value.Memberships), Bindings: accessBindingsResponse(value.Bindings),
		Denies: accessDeniesResponse(value.Denies),
	}
}

func accessUsersResponse(values []authzapp.AccessUser) []contract.AccessUser {
	result := make([]contract.AccessUser, 0, len(values))
	for _, value := range values {
		result = append(result, contract.AccessUser{Id: value.ID, Username: value.Username, Disabled: value.DisabledAt != nil, AllowedActions: accessActions(value.AllowedActions)})
	}
	return result
}

func accessUserCandidatesResponse(values []authzapp.AccessUser) []contract.AccessUserCandidate {
	result := make([]contract.AccessUserCandidate, 0, len(values))
	for _, value := range values {
		result = append(result, contract.AccessUserCandidate{Id: value.ID, Username: value.Username})
	}
	return result
}

func accessGroupsResponse(values []authzapp.AccessGroup) []contract.AccessGroup {
	result := make([]contract.AccessGroup, 0, len(values))
	for _, value := range values {
		result = append(result, accessGroupResponse(value))
	}
	return result
}

func accessRolesResponse(values []authzapp.AccessRole) []contract.AccessRole {
	result := make([]contract.AccessRole, 0, len(values))
	for _, value := range values {
		result = append(result, accessRoleResponse(value))
	}
	return result
}

func accessPermissionsResponse(values []authzapp.AccessPermission) []contract.AccessPermission {
	result := make([]contract.AccessPermission, 0, len(values))
	for _, value := range values {
		result = append(result, contract.AccessPermission{Key: value.Key, PlatformOnly: value.PlatformOnly, ProjectRoleDelegable: value.ProjectRoleDelegable})
	}
	return result
}

func accessMembershipsResponse(values []authzapp.AccessMembership) []contract.AccessMembership {
	result := make([]contract.AccessMembership, 0, len(values))
	for _, value := range values {
		result = append(result, accessMembershipResponse(value))
	}
	return result
}

func accessBindingsResponse(values []authzapp.AccessBinding) []contract.AccessBinding {
	result := make([]contract.AccessBinding, 0, len(values))
	for _, value := range values {
		result = append(result, accessBindingResponse(value))
	}
	return result
}

func accessDeniesResponse(values []authzapp.AccessDeny) []contract.AccessDeny {
	result := make([]contract.AccessDeny, 0, len(values))
	for _, value := range values {
		result = append(result, accessDenyResponse(value))
	}
	return result
}

func accessPrincipal(user identity.User) authzapp.AccessPrincipal {
	return authzapp.AccessPrincipal{UserID: user.ID, Disabled: user.Disabled}
}

func accessMutation(c *gin.Context) authzapp.AccessMutation {
	return authzapp.AccessMutation{RequestID: c.GetHeader(requestIDHeader)}
}

func accessGroupResponse(value authzapp.AccessGroup) contract.AccessGroup {
	return contract.AccessGroup{Id: value.ID, OwnerKind: contract.AccessGroupOwnerKind(value.OwnerKind), OwnerId: value.OwnerID, Name: value.Name, SystemKey: value.SystemKey, OidcViewerOnly: value.OIDCViewerOnly, Disabled: value.DisabledAt != nil, AllowedActions: accessActions(value.AllowedActions)}
}

func accessRoleResponse(value authzapp.AccessRole) contract.AccessRole {
	return contract.AccessRole{Id: value.ID, OwnerKind: contract.AccessRoleOwnerKind(value.OwnerKind), OwnerId: value.OwnerID, Name: value.Name, SystemKey: value.SystemKey, Active: value.Active, Permissions: value.Permissions, AllowedActions: accessActions(value.AllowedActions)}
}

func accessMembershipResponse(value authzapp.AccessMembership) contract.AccessMembership {
	return contract.AccessMembership{Id: value.ID, GroupId: value.GroupID, GroupName: value.GroupName, UserId: value.UserID, Username: value.Username, Source: contract.AccessMembershipSource(value.Source), Active: value.Active, AllowedActions: accessActions(value.AllowedActions)}
}

func accessBindingResponse(value authzapp.AccessBinding) contract.AccessBinding {
	return contract.AccessBinding{Id: value.ID, GroupId: value.GroupID, GroupName: value.GroupName, RoleId: value.RoleID, RoleName: value.RoleName, OrganizationId: accessUUIDPointer(value.OrganizationID), ScopeKind: contract.AccessBindingScopeKind(value.ScopeKind), ProjectId: accessUUIDPointer(value.ProjectID), EnvironmentId: value.EnvironmentID, ApplicationId: value.ApplicationID, Active: value.Active, AllowedActions: accessActions(value.AllowedActions)}
}

func accessDenyResponse(value authzapp.AccessDeny) contract.AccessDeny {
	return contract.AccessDeny{Id: value.ID, GroupId: value.GroupID, GroupName: value.GroupName, Permission: value.Permission, OrganizationId: value.OrganizationID, ScopeKind: contract.AccessDenyScopeKind(value.ScopeKind), ProjectId: value.ProjectID, EnvironmentId: value.EnvironmentID, ApplicationId: value.ApplicationID, Active: value.Active, AllowedActions: accessActions(value.AllowedActions)}
}

func accessActions(values []string) []contract.AccessAction {
	result := make([]contract.AccessAction, 0, len(values))
	for _, value := range values {
		result = append(result, contract.AccessAction(value))
	}
	return result
}

func accessPageMeta(c *gin.Context, next string, hasMore bool) contract.CursorPageMeta {
	meta := responseMeta(c)
	return contract.CursorPageMeta{RequestId: meta.RequestId, Timestamp: meta.Timestamp, NextCursor: optionalCursor(next), HasMore: hasMore}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringEnum[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func cursorValue(value *contract.Cursor) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func limitValue(value *contract.Limit) int {
	if value == nil {
		return 20
	}
	return int(*value)
}

func accessUUIDValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func accessUUIDPointer(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}
