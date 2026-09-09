package infrastructure

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"

	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
)

func TestAccessReadModelsIgnoreComputedSlicesDuringGORMScan(t *testing.T) {
	t.Parallel()

	models := map[string]any{
		"user":       &authzapp.AccessUser{},
		"group":      &authzapp.AccessGroup{},
		"role":       &authzapp.AccessRole{},
		"membership": &authzapp.AccessMembership{},
		"binding":    &authzapp.AccessBinding{},
		"deny":       &authzapp.AccessDeny{},
	}

	for name, model := range models {
		name, model := name, model
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("parse access read model: %v", err)
			}
			if field := parsed.LookUpField("AllowedActions"); field == nil || field.DBName != "" || field.Readable {
				t.Fatal("AllowedActions must not be mapped as a readable database field")
			}
			if name == "role" {
				if field := parsed.LookUpField("Permissions"); field == nil || field.DBName != "" || field.Readable {
					t.Fatal("Permissions must not be mapped as a readable database field")
				}
			}
		})
	}
}
