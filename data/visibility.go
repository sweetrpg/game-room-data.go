package data

import "github.com/sweetrpg/shelf-objects.go/models"

// CanView reports whether a viewer may see content at the given effective visibility level.
// Friendship isn't implemented yet, so callers always pass isFriend/isFriendOfFriend as false -
// this is the single code path that will start returning real friend-scoped results once
// friendship ships, per design.md.
func CanView(vis models.Visibility, ownerID, viewerID string, isFriend, isFriendOfFriend bool) bool {
	if viewerID != "" && viewerID == ownerID {
		return true
	}

	switch vis {
	case models.VisibilityPublic:
		return true
	case models.VisibilityFriendsOfFriends:
		return isFriend || isFriendOfFriend
	case models.VisibilityFriends:
		return isFriend
	default:
		return false
	}
}

// PreviewLibraryDefaultChange returns the volume IDs of entries that would become more exposed
// if the library's default visibility changed to newDefault. Entries with their own override are
// never affected by a default change, so only unoverridden entries are considered.
func PreviewLibraryDefaultChange(lib *models.Library, newDefault models.Visibility) []string {
	affected := make([]string, 0)
	for _, e := range lib.Entries {
		if e.VisibilityOverride != nil {
			continue
		}
		if newDefault.MoreExposedThan(lib.DefaultVisibility) {
			affected = append(affected, e.VolumeID)
		}
	}
	return affected
}
