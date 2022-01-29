{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    nativeBuildInputs = [ pkgs.gnumake pkgs.go pkgs.nsis ];
    CGO_ENABLED = "0";
}
