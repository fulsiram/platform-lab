{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    disko.url = "github:nix-community/disko";
    deploy-rs.url = "github:serokell/deploy-rs";
    sops-nix.url = "github:Mic92/sops-nix";

    kickstart.url = "path:./nix/dev-kickstart";
    kickstart.inputs.nixpkgs.follows = "nixpkgs";
    kickstart.inputs.disko.follows = "disko";
  };

  outputs =
    {
      self,
      nixpkgs,
      disko,
      deploy-rs,
      sops-nix,
      kickstart,
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

      # nixosConfigurations.devContainer = nixpkgs.lib.nixosSystem {
      #   system = "x86_64-linux";
      #   modules = [
      #     disko.nixosModules.disko
      #     ./nix/hosts/dev/configuration.nix
      #   ];
      # };

      nixosConfigurations.akaia = kickstart.nixosConfigurations.dev-kickstart.extendModules {
        modules = [
          ./nix/hosts/akaia/configuration.nix
          (
            {
              pkgs,
              lib,
              ...
            }:
            {
              system.bakedFlake = pkgs.runCommand "akaia-flake" { } ''
                cp -r ${kickstart}/. $out/
                chmod -R u+w $out
                cp ${./nix/hosts/akaia/configuration.nix} $out/configuration.nix
              '';
            }
          )
        ];
      };

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
