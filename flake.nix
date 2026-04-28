{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    deploy-rs.url = "github:serokell/deploy-rs";
    sops-nix.url = "github:Mic92/sops-nix";
  };

  outputs =
    {
      self,
      nixpkgs,
      disko,
      deploy-rs,
      sops-nix,
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

      nixosConfigurations.devContainer = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          ./nix/hosts/dev/configuration.nix
        ];
      };

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
