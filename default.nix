{ pkgs ? import <nixpkgs> {} }:

pkgs.callPackage ./nix/pgschema.nix {}
