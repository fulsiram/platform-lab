{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    base.url = "github:fulsiram/homelab?dir=nix/dev-base";
    base.inputs.nixpkgs.follows = "nixpkgs";
    base.inputs.disko.follows = "disko";

    continuity-os.url = "git+https://codeberg.org/Akaia_Collective/continuity_os?dir=system/nixos";

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
      continuity-os,
      ...
    }:
    {
      nixosConfigurations.system = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        specialArgs = { inherit inputs; };
        modules = [
          disko.nixosModules.disko
          base.nixosModules.base
          base.nixosModules.bakeFlake
          continuity-os.nixosModules.default
          "${akaia}/policy.nix"
          "${akaia}/configuration.nix"
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
          (if builtins.pathExists /etc/nixos/configuration.nix then /etc/nixos/configuration.nix else { })
        ];
      };

      packages.x86_64-linux.image = self.nixosConfigurations.system.config.system.build.diskoImages;
    };
}
