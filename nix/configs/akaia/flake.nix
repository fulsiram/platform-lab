{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    base.url = "github:fulsiram/homelab?dir=nix/dev-base";
    base.inputs.nixpkgs.follows = "nixpkgs";
    base.inputs.disko.follows = "disko";

    impermanence.url = "github:nix-community/impermanence";

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
      impermanence,
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
          impermanence.nixosModules.impermanence
          continuity-os.nixosModules.default
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
    };
}
