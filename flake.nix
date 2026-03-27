{
  description = "pgschema";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
          pgschema = pkgs.callPackage ./nix/pgschema.nix {
            rev = self.shortRev or "dirty";
            buildDate = self.lastModifiedDate or "unknown";
          };
        in
        {
          inherit pgschema;
          default = pgschema;
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.pgschema}/bin/pgschema";
        };
      });
    };
}
