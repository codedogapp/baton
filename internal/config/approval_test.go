package config_test

import (
	"testing"

	"github.com/codedogapp/baton/internal/config"
)

func TestLoadTurnsPlanApprovalOffAndOn(t *testing.T) {
	for content, want := range map[string]bool{
		`{}`:                      true,
		`{"planApproval": true}`:  true,
		`{"planApproval": false}`: false,
		`{"planApproval": null}`:  true,
		`{"planApproval": false, "repos": {"` + repo + `": {"planApproval": true}}}`: true,
		`{"planApproval": false, "repos": {"` + repo + `": {}}}`:                     false,
		`{"planApproval": false, "repos": {"` + repo + `": {"planApproval": null}}}`: false,
		`{"repos": {"` + repo + `": {"planApproval": false}}}`:                       false,
		`{"repos": {"github.com/codedogapp/tap": {"planApproval": false}}}`:          true,
	} {
		t.Run(content, func(t *testing.T) {
			got := load(t, content, remote)
			if got.PlanApproval == nil || *got.PlanApproval != want ||
				got.PlanApprovalOn() != want {
				t.Errorf("plan approval is %v (%v), want %v", got.PlanApproval,
					got.PlanApprovalOn(), want)
			}
		})
	}
}

func TestPlanApprovalIsOnWhereSettingsGiveNone(t *testing.T) {
	if !(config.Settings{}).PlanApprovalOn() {
		t.Error("plan approval is off in settings that give none, want on")
	}
}
