{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    nativeBuildInputs = [ pkgs.gnumake pkgs.go pkgs.nsis pkgs.zig pkgs.alsa-lib pkgs.pkg-config pkgs.ffmpeg-headless ];
}
