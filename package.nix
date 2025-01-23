{
  buildGoModule,
  lib,
  makeBinaryWrapper,
  mpv,
  rofi,
  ueberzugpp,
  withMpv ? true,
  withRofi ? false,
  withUeberzugpp ? false,
}: let
  inherit (lib) optional optionalString;

  path = optional withMpv mpv
    ++ optional withRofi rofi
    ++ optional withUeberzugpp ueberzugpp;
in
  buildGoModule {
    pname = "curd";
    version = builtins.readFile ./VERSION.txt;

    src = lib.fileset.toSource {
      root = ./.;
      fileset = lib.fileset.unions [
        ./cmd
        ./internal
        ./vendor

        ./go.mod
        ./go.sum
      ];
    };

    nativeBuildInputs = [makeBinaryWrapper];
    vendorHash = null;

    postFixup = optionalString (builtins.length path > 0) ''
      wrapProgram $out/bin/curd --prefix PATH : ${lib.makeBinPath path}
    '';

    meta = {
      description = "Watch anime in CLI with AniList Tracking, Discord RPC, and automatic intro/outro skipping";
      homepage = "https://github.com/Wraient/curd";
      license = lib.licenses.gpl3;
      platforms = lib.platforms.unix;
      maintainers = [lib.maintainers.diniamo];
      mainProgram = "curd";
    };
  }
