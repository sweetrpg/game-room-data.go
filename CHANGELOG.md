## 0.4.0 - 2026-08-30

### Added
- Snapshot volume title on entries and update it

### Fixed
- Scope write queries to the resource owner


## 0.3.0 - 2026-08-28

### Added
- Scope data access to wishlist ID, add list/create/delete and name migration


## 0.2.0 - 2026-08-28

### Added
- Stamp AddedAt on new library/wishlist entries


## 0.1.0 - 2026-08-25

### Added
- Add Shelf data access layer - library, wishlist, table repositories


### Changed
- Rename module from shelf-data.go to game-room-data.go


### Fixed
- Remove redundant embedded field selectors (staticcheck QF1008)