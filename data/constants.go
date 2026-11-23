package data

const (
	libraryCollection  = "libraries"
	wishlistCollection = "wishlists"
	tableCollection    = "tables"
	loanCollection     = "loans"

	// wishlistClaimCollection holds one document per (user, volume) that is on a wishlist, keyed by
	// that pair so a duplicate insert is the atomic "already on a wishlist" check.
	wishlistClaimCollection = "wishlist_volume_claims"
)

// DefaultWishlistName is the name assigned to a pre-existing single wishlist document by
// MigrateWishlistNames, and the default a caller may fall back to when creating a user's first
// wishlist without specifying one.
const DefaultWishlistName = "My Wishlist"
