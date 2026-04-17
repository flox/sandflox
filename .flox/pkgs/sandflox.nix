{ buildGoModule, lib }:

buildGoModule {
  pname = "sandflox";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../../.;
    fileset = lib.fileset.unions [
      (lib.fileset.fileFilter (file: lib.hasSuffix ".go" file.name) ../../.)
      ../../go.mod
      ../../templates  # //go:embed templates/*.tmpl -- shell enforcement templates
    ];
  };

  vendorHash = null;  # zero external dependencies (CORE-01)

  env.CGO_ENABLED = "0";  # static binary, no C dependencies

  buildFlags = [ "-trimpath" ];  # reproducible builds, no local path leaks

  ldflags = [
    "-s" "-w"
    "-X main.Version=0.1.0"
  ];

  # Skip tests during Nix build -- they run via `go test` in development
  doCheck = false;

  meta = with lib; {
    description = "macOS-native sandbox for AI coding agents";
    license = licenses.mit;
    platforms = platforms.darwin;
  };
}
