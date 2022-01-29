{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    nativeBuildInputs = [ pkgs.gnumake pkgs.go pkgs.nsis pkgs.zig ];
    CGO_ENABLED = "0";
}
