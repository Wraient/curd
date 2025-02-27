{
  description = "Watch anime in cli with Anilist Integration and Discord RPC ";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    systems.url = "github:nix-systems/default";
    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };
  };

  outputs = {
    nixpkgs,
    systems,
    flake-parts,
    ...
  } @ inputs:
    flake-parts.lib.mkFlake {inherit inputs;} {
      systems = import systems;

      perSystem = {pkgs, ...}: let
        package = pkgs.callPackage ./package.nix {};
      in {
        packages = {
          default = package;
          curd = package;
        };

        formatter = pkgs.alejandra;
        devShells.default = pkgs.mkShellNoCC {
          inputsFrom = [package];
        };
      };
    };
}
