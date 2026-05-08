{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    deploy-rs.url = "github:serokell/deploy-rs";
    sops-nix.url = "github:Mic92/sops-nix";

    akaia.url = "path:./nix/configs/akaia";
    akaia.inputs.nixpkgs.follows = "nixpkgs";
    akaia.inputs.disko.follows = "disko";
  };

  outputs =
    {
      self,
      nixpkgs,
      disko,
      deploy-rs,
      sops-nix,
      akaia,
    }:
    {
      nixosConfigurations.pepperoni = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          disko.nixosModules.disko
          ./nix/hosts/pepperoni/configuration.nix
          sops-nix.nixosModules.sops
          {
            sops.defaultSopsFile = ./secrets/secrets.yaml;
          }
        ];
      };

      deploy.nodes.pepperoni = {
        hostname = "pepperoni.salami.network";
        sshUser = "admin";
        remoteBuild = true;
        profiles.system = {
          user = "root";
          path = deploy-rs.lib.x86_64-linux.activate.nixos self.nixosConfigurations.pepperoni;
        };
      };

      nixosConfigurations.akaia = akaia.nixosConfigurations.system;
      packages.x86_64-linux.akaiaImages = self.nixosConfigurations.akaia.config.system.build.diskoImages;

      checks.x86_64-linux = deploy-rs.lib.x86_64-linux.deployChecks self.deploy;

      formatter.aarch64-darwin = nixpkgs.legacyPackages.aarch64-darwin.nixfmt;

      devShells.aarch64-darwin.default = nixpkgs.legacyPackages.aarch64-darwin.mkShell {
        packages = with nixpkgs.legacyPackages.aarch64-darwin; [
          nixos-rebuild
          nixfmt
          age
          kubeseal
          kustomize
        ];
      };
    };
}
