{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    {
      self,
      nixpkgs,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
    in
    {
      packages = nixpkgs.lib.genAttrs systems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          salami-cli = pkgs.buildGoModule {
            pname = "salami-cli";
            version = "0.1.0";

            src = ./salami-cli;
            subPackages = [ "cmd/salami/" ];
            meta.mainProgram = "salami";

            vendorHash = "sha256-z/gXXR+nBP5VOwyamNBkh3BjIIqx0CVDikL6khIuj9w=";
          };
        }
      );
    };
}
