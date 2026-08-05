package toolconfig

// Migrator rewrites a tool's on-disk config to the current ConfigVersion
// for non-major bumps. Refresh handles major bumps with a user-facing nudge
// to re-run activate; minor/patch bumps are silent and Migrate is responsible
// for stamping new defaults / fields onto the existing config.
//
// Migrate returns nil when the config is missing (nothing to migrate) and is
// expected to overwrite ConfigVersion / WrittenAt so the next scan sees the
// current schema version.
type Migrator interface {
	Tool() Tool
	Migrate(home string) error
}
