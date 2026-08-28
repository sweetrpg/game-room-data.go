package data

const (
	libraryCollection  = "libraries"
	wishlistCollection = "wishlists"
	tableCollection    = "tables"
)

// DefaultWishlistName is the name assigned to a pre-existing single wishlist document by
// MigrateWishlistNames, and the default a caller may fall back to when creating a user's first
// wishlist without specifying one.
const DefaultWishlistName = "My Wishlist"
