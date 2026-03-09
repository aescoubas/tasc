{ buildNpmPackage
, lib
, version ? "unstable"
}:

buildNpmPackage rec {
  pname = "tasc-web";
  inherit version;

  src = lib.cleanSource ../web;
  npmDepsHash = "sha256-6wH/HdF0Er5gbNd8v6OgHmLbSI/HMS8hHxRVI3Tj724=";

  installPhase = ''
    runHook preInstall
    mkdir -p "$out"
    cp -r dist/* "$out/"
    runHook postInstall
  '';

  meta = with lib; {
    description = "React dashboard for Tasc";
    homepage = "https://github.com/aescoubas/tasc";
    license = licenses.mpl20;
    platforms = platforms.all;
  };
}
