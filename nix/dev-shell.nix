{ go_1_24
, gopls
, goreleaser
, lib
, mkShell
, nixfmt
, nodejs
, pkg-config
, sqlite
, stdenv
, tasc
}:

mkShell {
  inputsFrom = [ tasc ];

  packages = [
    go_1_24
    gopls
    goreleaser
    nixfmt
    nodejs
    sqlite
    pkg-config
  ] ++ lib.optionals stdenv.hostPlatform.isLinux [
    stdenv.cc
  ];

  shellHook = ''
    export CGO_ENABLED=1
  '';
}
