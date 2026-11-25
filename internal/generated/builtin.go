package generated

// Builtin is clump's list of paths that are not worth reviewing line by
// line: files a tool writes (lock files, codegen output, migration
// snapshots, minified bundles) and vendored dependencies. A change to one
// of them follows from a change elsewhere, which is what gets reviewed.
//
// Handwritten-but-generated-once files (Django migrations, Jest snapshots,
// CHANGELOG.md, go.mod) are deliberately absent: their diffs carry intent.
// Add them with `patterns` in the config if you disagree.
var Builtin = []string{
	// Dependency lock files.
	"package-lock.json",
	"npm-shrinkwrap.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"bun.lock",
	"bun.lockb",
	"deno.lock",
	"Cargo.lock",
	"go.sum",
	"go.work.sum",
	"Gopkg.lock",
	"poetry.lock",
	"Pipfile.lock",
	"uv.lock",
	"pdm.lock",
	"composer.lock",
	"Gemfile.lock",
	"mix.lock",
	"flake.lock",
	"pubspec.lock",
	"Podfile.lock",
	"Package.resolved",
	"Cartfile.resolved",
	"packages.lock.json",
	"gradle.lockfile",
	"MODULE.bazel.lock",
	"Chart.lock",
	"renv.lock",
	".terraform.lock.hcl",

	// Yarn Plug'n'Play and vendored dependencies.
	".pnp.cjs",
	".pnp.loader.mjs",
	"**/.yarn/releases/",
	"**/.yarn/plugins/",
	"**/.yarn/sdks/",
	"**/.yarn/cache/",
	"vendor/",
	"node_modules/",
	"Pods/",

	// Minified bundles and source maps.
	"*.min.js",
	"*.min.mjs",
	"*.min.css",
	"*.js.map",
	"*.mjs.map",
	"*.cjs.map",
	"*.css.map",
	"*.d.ts.map",

	// Framework build output and type stubs.
	"next-env.d.ts",
	".next/",
	".nuxt/",
	".svelte-kit/",
	".astro/",

	// Generated directories and file-name conventions.
	"__generated__/",
	"*.generated.*",
	"*.gen.*",
	"*_gen.go",
	"zz_generated*",

	// Protocol Buffers / gRPC.
	"*.pb.go",
	"*.pb.gw.go",
	"*.pb.cc",
	"*.pb.h",
	"*.pb.swift",
	"*_pb2.py",
	"*_pb2.pyi",
	"*_pb2_grpc.py",
	"*_pb.js",
	"*_pb.d.ts",
	"*_pb.ts",

	// GraphQL codegen (client preset, gql.tada).
	"**/gql/graphql.ts",
	"**/gql/gql.ts",
	"**/gql/fragment-masking.ts",
	"graphql-env.d.ts",

	// Database: ORM migrations and schema dumps written by tools.
	"**/drizzle/**/*.sql",
	"**/meta/_journal.json",
	"**/meta/*_snapshot.json",
	"**/migrations/*/snapshot.json", // drizzle-kit v1: <ts>_<name>/snapshot.json
	"**/prisma/migrations/**/migration.sql",
	"**/prisma/migrations/migration_lock.toml",
	"**/db/schema.rb",
	"**/db/structure.sql",
	"database.types.ts",

	// Dart / Flutter build_runner, .NET designers.
	"*.g.dart",
	"*.freezed.dart",
	"*.mocks.dart",
	"*.gr.dart",
	"*.Designer.cs",
	"*.designer.cs",
	"*.g.cs",
	"*.g.i.cs",
}
