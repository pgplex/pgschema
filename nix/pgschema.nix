{ pkgs ? import <nixpkgs> {}, rev ? "unknown", buildDate ? "unknown" }:

let
  lib = pkgs.lib;
  version = lib.strings.removeSuffix "\n" (builtins.readFile ../internal/version/VERSION);
in
pkgs.buildGoModule {
  pname = "pgschema";
  inherit version;

  src = lib.cleanSource ../.;
  # Prefer Go 1.24 when available; fall back to the closest newer toolchain.
  go = if pkgs ? go_1_24 then pkgs.go_1_24 else pkgs.go_1_25;
  subPackages = [ "." ];
  proxyVendor = true;
  # buildGoModule runs `go test ./...` by default; disable checks because
  # this repository's integration test setup starts embedded Postgres, which
  # is not reliable in Nix's sandboxed build environment.
  doCheck = false;

  # Update if `nix build` reports a vendorHash mismatch.
  vendorHash = "sha256-3nV7AEsWyEvIbxHetoEsA8PPXJ6ENvU/sz7Wn5aysss=";

  env = {
    CGO_ENABLED = "0";
  };
  ldflags = [
    "-s"
    "-w"
    "-X"
    "github.com/pgplex/pgschema/cmd.GitCommit=${rev}"
    "-X"
    "github.com/pgplex/pgschema/cmd.BuildDate=${buildDate}"
  ];

  meta = with lib; {
    description = "PostgreSQL schema management tool";
    homepage = "https://www.pgschema.com";
    license = licenses.asl20;
    mainProgram = "pgschema";
    platforms = platforms.unix;
  };
}
