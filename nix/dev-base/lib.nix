{ pkgs, lib, ... }:
{
  mkBakedFlake =
    {
      source,
      files ? [ ],
    }:
    pkgs.runCommand "baked-flake" { } (
      ''
        mkdir -p $out
        cp ${source}/flake.nix  $out/flake.nix
        cp ${source}/flake.lock $out/flake.lock
      ''
      + lib.concatMapStrings (f: ''
        mkdir -p "$out/$(dirname ${f})"
        cp ${source}/${f} "$out/${f}"
      '') files
    );
}
