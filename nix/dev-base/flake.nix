{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
  };

  outputs =
    inputs@{
      self,
      pkgs,
      nixpkgs,
      disko,
    }:
    {
      nixosModules = {
        base = ./modules/base.nix;
        diskLayout = ./modules/disk-config.nix;
        bakeFlake = ./modules/bake-flake.nix;
      };

      lib = import ./lib.nix {
        pkgs = pkgs;
        lib = nixpkgs.lib;
      };

      nixosConfigurations.base = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        specialArgs = { inherit inputs; };
        modules = [
          disko.nixosModules.disko
          self.nixosModules.diskLayout
          self.nixosModules.base
          self.nixosModules.bakeFlake
          (
            { lib, ... }:
            {
              system.bakedFlake = self;
            }
          )
        ];
      };

      packages.x86_64-linux.image = self.nixosConfigurations.base.config.system.build.diskoImages;
    };
}
