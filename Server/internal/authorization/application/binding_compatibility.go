package application

func roleHasEffectivePermission(role AccessRole, permissions []AccessPermission, scopeKind string) bool {
	metadata := make(map[string]AccessPermission, len(permissions))
	for _, permission := range permissions {
		metadata[permission.Key] = permission
	}
	for _, key := range role.Permissions {
		permission, ok := metadata[key]
		if !ok {
			continue
		}
		if scopeKind == "platform" && permission.PlatformOnly {
			return true
		}
		if scopeKind != "platform" && !permission.PlatformOnly {
			return true
		}
	}
	return false
}
