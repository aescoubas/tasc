{ buildGoModule
, go_1_24
, installShellFiles
, lib
, stdenv
, version ? "unstable"
}:

buildGoModule rec {
  pname = "tasc";
  inherit version;

  src = lib.cleanSourceWith {
    src = ../.;
    filter = path: type:
      let
        relPath = lib.removePrefix "${toString ../.}/" (toString path);
      in
      lib.cleanSourceFilter path type
      && !(lib.hasPrefix "dist/" relPath)
      && !(lib.hasPrefix "docs/" relPath)
      && !(lib.hasPrefix "web/" relPath);
  };

  vendorHash = "sha256-3FpOIUs+7aey4wwnH/BZU7JAr/vIx4/qhztOGvfbXDI=";
  go = go_1_24;
  subPackages = [ "." ];
  tags = [ "fts5" ];

  env.CGO_ENABLED = 1;

  nativeBuildInputs = [ installShellFiles ];

  checkFlags = [ "-tags=fts5" ];

  postInstall = ''
    export HOME="$TMPDIR/home"
    export TASC_DB_PATH="$TMPDIR/tasc.db"
    mkdir -p "$HOME"
    mkdir -p completions
    "$out/bin/tasc" completion bash > completions/tasc.bash
    "$out/bin/tasc" completion zsh > completions/_tasc
    installShellCompletion --cmd tasc \
      --bash completions/tasc.bash \
      --zsh completions/_tasc
  '';

  meta = with lib; {
    description = "A modern, snappy, and powerful CLI task manager built for the terminal";
    homepage = "https://github.com/aescoubas/tasc";
    license = licenses.mpl20;
    mainProgram = "tasc";
    platforms = platforms.linux ++ platforms.darwin;
  };
}
