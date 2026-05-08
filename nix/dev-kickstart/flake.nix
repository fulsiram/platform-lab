{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    base.url = "github:fulsiram/homelab?dir=nix/dev-base";
    base.inputs.nixpkgs.follows = "nixpkgs";
    base.inputs.disko.follows = "disko";
  };

  outputs =
    inputs@{
      self,
      nixpkgs,
      disko,
      base,
      ...
    }:
    {
      nixosConfigurations.dev-kickstart = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        specialArgs = { inherit inputs; };
        modules = [
          disko.nixosModules.disko
          base.nixosModules.base
          base.nixosModules.bakeFlake
          (
            { lib, ... }:
            {
              system.bakedFlake = lib.mkDefault self;
            }
          )
          (if builtins.pathExists /etc/nixos/configuration.nix then /etc/nixos/configuration.nix else { })
        ];
      };

      packages.x86_64-linux.image =
        self.nixosConfigurations.dev-kickstart.config.system.build.diskoImages;
    };
}
