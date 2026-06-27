//go:build integration

package message

// Discriminating integration tests for the PRODUCTION DM cross-Space leak
// reported in dm-space-leak-report (user 5ebd42… saw 明略 DMs in the 温州版权馆
// "最近" list). The forensic report offers ONE explanation — the client request
// omitted space_id so the Space filter never ran — but does not capture the real
// request, and an alternative (the request carried the user's DEFAULT Space, and
// the decideConvKeepInSpace catch-all listed every DM there) produces the same
// "lots of DMs" outcome. These two tests drive the REAL /v1/conversation/sync
// handler to characterise BOTH mechanisms in code, so the candidates can be told
// apart from a real request without a production capture.
//
// Mapping to the forensic snapshot:
//
//	reproSpaceDefault  ⇆ minglue_default            (user's DEFAULT Space)
//	reproSpaceB        ⇆ 1678…020c (温州版权馆)       (the Space being viewed)
//	reproSpaceC        ⇆ a third non-default Space   (isolation contrast)
//
// Peer channel_ids are REAL ids lifted from dm_analysis.json:
//
//	prodPeerHumanX/Y/Z  human DMs (no user.robot row)
//	prodPeerRobot        balalaxmx_bot — user.robot=1, msgs untagged, not a 温州
//	                     member; the report's "实锤" that a DM any real filter MUST
//	                     drop was nonetheless visible.
//
// Neither mechanism is touched by the #484 fix (the missing-space_id guard lives
// in the handler; the catch-all is the pre-existing 193acfe logic), so these
// pass independent of that change — no dm_space_presence rows are written here.
//
//	go test -tags=integration ./modules/message/ -run TestRepro484_ProdTrace -v

import (
	"testing"

	"github.com/Mininglamp-OSS/octo-lib/config"
	"github.com/stretchr/testify/assert"
)

const (
	prodPeerHumanX = "63703cde3ba7466dae1801d9caec2db2"
	prodPeerHumanY = "9533bab2300e4fe080eae53ff633de2a"
	prodPeerHumanZ = "73326bba740946eda8b42eb6e287f8a5"
	prodPeerRobot  = "balalaxmx_bot"
)

// TestRepro484_ProdTrace_MissingSpaceID_LeaksCrossSpaceDMs reproduces the FIRST
// mechanism: the request carried NO space context. SpaceMiddleware fails open
// (pkg/space/middleware.go:108-115 → c.Next() without setting space_id), so the
// handler's `if spaceID != ""` guard skips FilterConversationsBySpace entirely
// (api_conversation.go:889) and returns the user's DMs from EVERY Space.
//
// Discriminator: with identical data, the only difference between the two calls
// is the presence of the space header. The DMs a 温州 (non-default) query
// correctly drops are returned when the header is omitted — including the
// untagged robot DM (balalaxmx_bot), matching the production screenshot.
func TestRepro484_ProdTrace_MissingSpaceID_LeaksCrossSpaceDMs(t *testing.T) {
	s, ctx := reproSetup(t)
	// balalaxmx is a robot (user.robot=1) but a member of NO Space here and its
	// messages carry no space_id → any real filter MUST drop it under 温州.
	reproSeedBotUser(t, ctx, prodPeerRobot)

	// A production-shaped DM set: two human DMs whose messages are tagged only
	// with the DEFAULT Space (minglue), one untagged robot DM, and one DM that
	// legitimately belongs to 温州 (spaceB).
	reproIMConvs = []*config.SyncUserConversationResp{
		reproDMConvFor(prodPeerHumanX, "minglue-X", reproSpaceDefault),
		reproDMConvFor(prodPeerHumanY, "minglue-Y", reproSpaceDefault),
		reproDMConvFor(prodPeerRobot, "robot-untagged", ""),
		reproDMConvFor(prodPeerHumanZ, "wenzhou-real", reproSpaceB),
	}

	// (1) No space context → filter skipped → every DM returned.
	noSpace := reproCallConvSyncNoSpace(t, s)
	assert.True(t, reproContains(noSpace, prodPeerHumanX), "no space_id: a minglue-only DM leaks")
	assert.True(t, reproContains(noSpace, prodPeerHumanY), "no space_id: a minglue-only DM leaks")
	assert.True(t, reproContains(noSpace, prodPeerRobot),
		"no space_id: the untagged robot DM leaks too (matches the report's balalaxmx 实锤)")
	assert.True(t, reproContains(noSpace, prodPeerHumanZ), "no space_id: the 温州 DM is also present")

	// (2) Same data, now WITH X-Space-ID=温州 → filter runs → cross-Space dropped.
	wenzhou := reproCallConvSync(t, s, reproSpaceB)
	assert.True(t, reproContains(wenzhou, prodPeerHumanZ), "温州 query keeps the DM that belongs to 温州")
	assert.False(t, reproContains(wenzhou, prodPeerHumanX), "温州 query drops the minglue-only DM")
	assert.False(t, reproContains(wenzhou, prodPeerHumanY), "温州 query drops the minglue-only DM")
	assert.False(t, reproContains(wenzhou, prodPeerRobot), "温州 query drops the untagged robot DM")

	// The delta between (1) and (2) — identical data, only the space header
	// differs — isolates "Space filter not executed" as the leak's direct cause.
}

// TestRepro484_ProdTrace_DefaultSpaceCatchAll_ShowsAllDMs reproduces the SECOND
// candidate: the request carried the user's DEFAULT Space. decideConvKeepInSpace's
// catch-all (space_filter.go:305-309) returns true for EVERY bare non-bot DM when
// filterSpaceID == defaultSpaceID — so the default Space lists DMs that belong
// only to OTHER Spaces.
//
// But this mechanism has a TELL distinguishing it from the missing-space_id path:
// the catch-all still runs the bot sub-check, so a robot DM that is NOT a member
// of the default Space is HIDDEN. balalaxmx therefore does NOT appear under the
// default-Space query — yet the production screenshot showed it. So the catch-all
// alone cannot explain the report; the missing-space_id path (test above) fits
// the evidence better. Capturing one real request decides between them.
func TestRepro484_ProdTrace_DefaultSpaceCatchAll_ShowsAllDMs(t *testing.T) {
	s, ctx := reproSetup(t)
	reproSeedBotUser(t, ctx, prodPeerRobot) // robot, member of NO Space here

	reproIMConvs = []*config.SyncUserConversationResp{
		reproDMConvFor(prodPeerHumanX, "only-wenzhou", reproSpaceB), // human, msgs only in 温州
		reproDMConvFor(prodPeerHumanY, "only-spaceC", reproSpaceC),  // human, msgs only in spaceC
		reproDMConvFor(prodPeerRobot, "robot-untagged", ""),         // robot, untagged
	}

	// Query the DEFAULT Space → catch-all lists cross-Space HUMAN DMs...
	def := reproCallConvSync(t, s, reproSpaceDefault)
	assert.True(t, reproContains(def, prodPeerHumanX),
		"catch-all: a 温州-only DM shows in the default Space")
	assert.True(t, reproContains(def, prodPeerHumanY),
		"catch-all: a spaceC-only DM shows in the default Space")
	// ...but the catch-all bot sub-check HIDES a robot that is not a default member.
	assert.False(t, reproContains(def, prodPeerRobot),
		"catch-all hides a non-member robot — so this mechanism alone can't explain balalaxmx leaking in prod")

	// Contrast: a non-default Space isolates correctly (no catch-all).
	spaceC := reproCallConvSync(t, s, reproSpaceC)
	assert.True(t, reproContains(spaceC, prodPeerHumanY), "spaceC query keeps the spaceC-only DM")
	assert.False(t, reproContains(spaceC, prodPeerHumanX), "spaceC query drops the 温州-only DM")
}
