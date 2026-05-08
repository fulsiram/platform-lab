{
  modulesPath,
  pkgs,
  lib,
  inputs,
  ...
}:
{
  imports = [
    "${modulesPath}/profiles/qemu-guest.nix"
    ./disk-config.nix
  ];

  boot.growPartition = true;
  boot.kernelParams = [ "console=ttyS0" ];
  boot.loader.timeout = 0;

  boot.loader.grub.efiSupport = true;
  boot.loader.grub.efiInstallAsRemovable = true;
  boot.loader.grub.device = "/dev/vda";

  services.qemuGuest.enable = true;
  services.openssh.enable = true;
  services.cloud-init.enable = true;
  systemd.services."serial-getty@ttyS0".enable = true;

  systemd.services.grow-partitions = {
    wantedBy = [ "multi-user.target" ];
    after = [ "nix.mount" ];
    requires = [ "nix.mount" ];

    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      SuccessExitStatus = "0 1";
    };

    path = with pkgs; [
      cloud-utils.guest
      e2fsprogs
    ];

    script = ''
      growpart /dev/disk/by-id/virtio-NIXSTORE 1 || true;
      resize2fs /dev/disk/by-id/virtio-NIXSTORE-part1
    '';
  };

  nix.settings.experimental-features = [
    "nix-command"
    "flakes"
  ];
  nix.registry = lib.mapAttrs (_: v: { flake = v; }) inputs;
  nix.nixPath = [ "nixpkgs=${inputs.nixpkgs}" ];
  system.extraDependencies = lib.attrValues inputs;
}
