package metrics

import (
	"strings"
	"testing"
)

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
				Version:   "1.+",
				Endpoint:  "gradle-analytics.services.bitrise.io",
				AuthToken: "example_token",
				Port:      443,
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
			gotTrimmed := trimAllWhitespace(got)
			wantTrimmed := trimAllWhitespace(tt.want)
			if gotTrimmed != wantTrimmed {
				t.Errorf("renderTemplate() got = %v, want %v", gotTrimmed, wantTrimmed)
			}
		})
	}
}

func trimAllWhitespace(s string) string {
	trimmed := strings.ReplaceAll(s, "\n", "")
	trimmed = strings.ReplaceAll(trimmed, "\t", "")
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed
}

const expectedInitScript = `
initscript {
    repositories {
        maven {
            url 'https://plugins.gradle.org/m2/'
        }
        mavenCentral()
    }
    dependencies {
        classpath 'io.bitrise.gradle:gradle-analytics:1.+'
    }
}

rootProject {
    apply plugin: io.bitrise.gradle.analytics.AnalyticsPlugin

    analytics {
        ignoreErrors = false
        bitrise {
            remote {
                authorization = 'example_token'
                endpoint = 'gradle-analytics.services.bitrise.io'
                port = 443
			}
        }
    }
}
`
