{
  description = "Watch anime in cli with Anilist Integration and Discord RPC ";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    systems.url = "github:nix-systems/default";
  };

  outputs = {
    nixpkgs,
    systems,
    ...
  }: {
    packages = nixpkgs.lib.genAttrs (import systems) (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};
        package = pkgs.callPackage ./package.nix {};
      in {
        default = package;
        curd = package;
      }
    );
  };
}
