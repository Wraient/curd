{
  description = "Watch anime in cli with Anilist Integration and Discord RPC ";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    systems.url = "github:nix-systems/default";
  };

  outputs = {
    nixpkgs,
    systems,
    self,
    ...
  }: let
    eachSystem = nixpkgs.lib.genAttrs (import systems);
  in {
    packages = eachSystem (system: let
      package = nixpkgs.legacyPackages.${system}.callPackage ./package.nix {};
    in {
      default = package;
      curd = package;
    });

    devShells = eachSystem (system: {
      default = nixpkgs.legacyPackages.${system}.mkShellNoCC {
        inputsFrom = [self.packages.${system}.default];
      };
    });

    formatter = eachSystem (system: nixpkgs.legacyPackages.${system}.alejandra);
  };
}
