{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    base.url = "github:fulsiram/homelab?dir=nix/dev-base";
    base.inputs.nixpkgs.follows = "nixpkgs";
    base.inputs.disko.follows = "disko";

    impermanence.url = "github:nix-community/impermanence";

    dstk.url = "git+https://codeberg.org/Akaia_Collective/dstk?dir=projects/continuity_os/system";

    akaia = {
      url = "github:fulsiram/homelab?dir=nix/configs/akaia";
      flake = false;
    };
  };

  outputs =
    inputs@{
      self,
      nixpkgs,
      disko,
      base,
      akaia,
      dstk,
      impermanence,
      ...
    }:
    {
      nixosConfigurations.nixos = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        specialArgs = { inherit inputs; };
        modules = [
          disko.nixosModules.disko
          base.nixosModules.base
          base.nixosModules.bakeFlake
          impermanence.nixosModules.impermanence
          dstk.nixosModules.fullNode
          "${akaia}/policy.nix"
          ./configuration.nix
          (
            { lib, ... }:
            {
              system.bakedFlake = lib.mkDefault (
                base.lib.mkBakedFlake {
                  source = self;
                  files = [ "configuration.nix" ];
                }
              );
            }
          )
        ];
      };

      packages.x86_64-linux.images = self.nixosConfigurations.system.config.system.build.diskoImages;
    };
}
