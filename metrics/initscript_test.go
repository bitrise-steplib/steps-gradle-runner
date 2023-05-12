package metrics

import "testing"

func Test_renderTemplate(t *testing.T) {
	tests := []struct {
		name      string
		inventory templateInventory
		want      string
		wantErr   bool
	}{
		{
			name: "happy path",
			inventory: templateInventory{
				Version:         "1.+",
				Endpoint:        "grpcs://example.com",
				AuthToken:       "example_token",
				PushEnabled:     true,
				DebugEnabled:    true,
				ValidationLevel: "error",
			},
			want: expectedInitScript,
		},
		{
			name: "invalid endpoint",
			inventory: templateInventory{
				Version:   "1.0.0",
				Endpoint:  "",
				AuthToken: "example_token",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(tt.inventory)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("renderTemplate() got = %v, want %v", got, tt.want)
			}
		})
	}
}

const expectedInitScript = `initscript {
    repositories {
        mavenCentral()
        maven {
            url 'https://jitpack.io'
        }
    }

    dependencies {
        classpath 'io.bitrise.gradle:remote-cache:1.+'
    }
}

import io.bitrise.gradle.cache.BitriseBuildCache
import io.bitrise.gradle.cache.BitriseBuildCacheServiceFactory

gradle.settingsEvaluated { settings ->
    settings.buildCache {
        local {
            enabled = false
        }

        registerBuildCacheService(BitriseBuildCache.class, BitriseBuildCacheServiceFactory.class)
        remote(BitriseBuildCache.class) {
            endpoint = 'grpcs://example.com'
            authToken = 'example_token'
            enabled = true
            push = true
            debug = true
            blobValidationLevel = 'error'
        }
    }
}
`
