{
  description = "A CLI anime streaming tool with AniList integration";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }: 
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      
      # Helper function to generate attrsets for multiple systems
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      
      # Helper function to get pkgs for each system
      pkgsFor = system: import nixpkgs { inherit system; };
      
    in {
      packages = forAllSystems (system: let 
        pkgs = pkgsFor system;
      in {
        default = self.packages.${system}.curd;  # Default Package
        
        curd = pkgs.stdenv.mkDerivation rec {
          pname = "curd";
          version = "1.0.5";
          
          src = pkgs.fetchurl {
            url = "https://github.com/Wraient/curd/releases/download/v${version}/curd";
            sha256 = "1rkdy6p13i4213hhx7s08ix75in0xjpfvpxcsbjrpdlr14kc044s";
          };

          nativeBuildInputs = [ pkgs.makeWrapper ];
          
          buildInputs = with pkgs; [ 
            mpv 
            ueberzugpp
            rofi
          ];

          phases = ["installPhase"];
          
          installPhase = ''
            install -Dm755 $src $out/bin/curd
            wrapProgram $out/bin/curd \
              --prefix PATH : ${pkgs.lib.makeBinPath buildInputs}
          '';

          meta = with pkgs.lib; {
            description = "Watch anime in CLI with AniList Tracking, Discord RPC, and automatic intro/outro skipping";
            homepage = "https://github.com/Wraient/curd";
            license = licenses.gpl3;
            platforms = platforms.linux;
            mainProgram = "curd";
          };
        };
      });

      # Add apps to allow use with `nix run`
      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.curd}/bin/curd";
        };
      });
    };
}
