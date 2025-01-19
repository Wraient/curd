{
  description = "Curd Flake";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  outputs = { self, nixpkgs }: {
    packages.x86_64-linux = let
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in {
      curd = pkgs.stdenv.mkDerivation {
        pname = "curd";
        version = "1.0.5";
        src = pkgs.fetchurl {
          url = "https://github.com/Wraient/curd/releases/download/v1.0.5/curd";
          sha256 = "1rkdy6p13i4213hhx7s08ix75in0xjpfvpxcsbjrpdlr14kc044s";
        };
        nativeBuildInputs = [ pkgs.makeWrapper ];
        buildInputs = [ pkgs.mpv ];
        phases = ["installPhase"];
        installPhase = ''
          install -Dm755 $src $out/bin/curd
          wrapProgram $out/bin/curd \
            --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.mpv ]}
        '';
        meta = with pkgs.lib; {
          description = "Watch anime in CLI with AniList Tracking, Discord RPC, Intro/Outro/Filler/Recap Skipping, etc";
          homepage = "https://github.com/Wraient/curd";
          license = licenses.gpl3;
          platforms = platforms.linux;
        };
      };
    };
  };
}
