package data

import (
	"testing"

	"github.com/sweetrpg/game-room-objects.go/models"
)

func TestCanView(t *testing.T) {
	cases := []struct {
		name             string
		vis              models.Visibility
		ownerID          string
		viewerID         string
		isFriend         bool
		isFriendOfFriend bool
		want             bool
	}{
		{"owner always sees own private", models.VisibilityPrivate, "u1", "u1", false, false, true},
		{"public visible to anonymous", models.VisibilityPublic, "u1", "", false, false, true},
		{"public visible to any other user", models.VisibilityPublic, "u1", "u2", false, false, true},
		{"private hidden from other user", models.VisibilityPrivate, "u1", "u2", false, false, false},
		{"private hidden from anonymous", models.VisibilityPrivate, "u1", "", false, false, false},
		{"friends hidden from non-friend before friendship ships", models.VisibilityFriends, "u1", "u2", false, false, false},
		{"friends visible to friend", models.VisibilityFriends, "u1", "u2", true, false, true},
		{"friends-of-friends hidden from stranger", models.VisibilityFriendsOfFriends, "u1", "u2", false, false, false},
		{"friends-of-friends visible to friend-of-friend", models.VisibilityFriendsOfFriends, "u1", "u2", false, true, true},
		{"friends-of-friends visible to direct friend too", models.VisibilityFriendsOfFriends, "u1", "u2", true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanView(c.vis, c.ownerID, c.viewerID, c.isFriend, c.isFriendOfFriend); got != c.want {
				t.Errorf("CanView(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestPreviewLibraryDefaultChange(t *testing.T) {
	friends := models.VisibilityFriends
	lib := &models.Library{
		DefaultVisibility: models.VisibilityPrivate,
		Entries: []models.LibraryEntry{
			{VolumeID: "no-override"},
			{VolumeID: "has-override", VisibilityOverride: &friends},
		},
	}

	affected := PreviewLibraryDefaultChange(lib, models.VisibilityPublic)
	if len(affected) != 1 || affected[0] != "no-override" {
		t.Fatalf("widening private->public: got %v, want [no-override]", affected)
	}

	narrower := PreviewLibraryDefaultChange(lib, models.VisibilityPrivate)
	if len(narrower) != 0 {
		t.Fatalf("no-op change private->private: got %v, want none", narrower)
	}
}
