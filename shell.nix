{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    nativeBuildInputs = [ pkgs.gnumake pkgs.go ];
    CGO_ENABLED = "0";
}
