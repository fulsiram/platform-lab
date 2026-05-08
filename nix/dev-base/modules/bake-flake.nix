{ lib, config, ... }:
{
  options.system.bakedFlake = lib.mkOption {
    type = lib.types.nullOr lib.types.path;
    default = null;
    description = "Flake source to drop at /etc/nixos on first boot.";
  };

  config = lib.mkIf (config.system.bakedFlake != null) {
    systemd.tmpfiles.rules = [
      "C /etc/nixos 0755 root root - ${config.system.bakedFlake}"
    ];
    environment.etc."nixos-base".source = config.system.bakedFlake;
  };
}
