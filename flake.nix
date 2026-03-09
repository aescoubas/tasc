{
  description = "Nix-native build for the tasc CLI and development tooling";

  inputs = {
    flake-utils.url = "github:numtide/flake-utils";
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, flake-utils, nixpkgs }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };

        version =
          if self ? shortRev then "unstable-${self.shortRev}"
          else if self ? dirtyShortRev then "unstable-${self.dirtyShortRev}"
          else "unstable";

        tasc = pkgs.callPackage ./nix/package.nix {
          inherit version;
        };

        web = pkgs.callPackage ./nix/web.nix {
          inherit version;
        };
      in
      {
        packages = {
          default = tasc;
          tasc = tasc;
          web = web;
        };

        apps = {
          default = flake-utils.lib.mkApp {
            drv = tasc;
          };
          tasc = flake-utils.lib.mkApp {
            drv = tasc;
          };
        };

        checks = {
          cli = tasc;
          web = web;
          default = pkgs.linkFarm "tasc-checks" [
            {
              name = "cli";
              path = tasc;
            }
            {
              name = "web";
              path = web;
            }
          ];
        };

        devShells.default = pkgs.callPackage ./nix/dev-shell.nix {
          inherit tasc;
        };

        formatter = pkgs.nixfmt;
      });
}
