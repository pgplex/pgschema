{ pkgs ? import <nixpkgs> {} }:

let
  lib = pkgs.lib;
  version = lib.strings.removeSuffix "\n" (builtins.readFile ../internal/version/VERSION);
in
pkgs.buildGoModule {
  pname = "pgschema";
  inherit version;

  src = lib.cleanSource ../.;
  # go_1_24 is not available in some nixpkgs revisions; use the closest newer toolchain.
  go = pkgs.go_1_25;
  subPackages = [ "." ];
  proxyVendor = true;

  # Replace with the real hash from `nix build` output.
  vendorHash = "sha256-3nV7AEsWyEvIbxHetoEsA8PPXJ6ENvU/sz7Wn5aysss=";

  env = {
    CGO_ENABLED = "0";
  };
  ldflags = [
    "-s"
    "-w"
  ];

  meta = with lib; {
    description = "PostgreSQL schema management tool";
    homepage = "https://www.pgschema.com";
    license = licenses.asl20;
    mainProgram = "pgschema";
    platforms = platforms.unix;
  };
}
