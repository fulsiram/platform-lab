{
  modulesPath,
  pkgs,
  lib,
  inputs,
  ...
}:
{
  imports = [
    # "${modulesPath}/profiles/qemu-guest.nix"
    ./disk-config.nix
  ];

  boot.initrd.kernelModules = [
    "virtio_mmio"
    "virtio_pci"
    "virtio_blk"
    "virtiofs"
  ];

  boot.blacklistedKernelModules = [
    "rfkill"
    "intel_pstate"
  ];

  boot.initrd.systemd.tpm2.enable = false;
  systemd.tpm2.enable = false;
  boot.swraid.enable = false;

  boot.growPartition = false;
  systemd.services.grow-partitions.enable = false;

  boot.kernelParams = [
    "console=ttyS0"
    "8250.nr_uarts=1"
  ];
  boot.loader.timeout = 0;
  boot.loader.grub.enable = false;
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = false;

  # boot.loader.grub.efiSupport = true;
  # boot.loader.grub.efiInstallAsRemovable = true;
  # boot.loader.grub.device = "nodev";

  boot.initrd.includeDefaultModules = false;
  boot.initrd.availableKernelModules = [
    "ahci"
    "virtio_pci"
    "virtio_scsi"
    "virtio_blk"
  ];

  services.qemuGuest.enable = true;
  services.openssh.enable = true;
  services.cloud-init.enable = false;
  systemd.services."serial-getty@ttyS0".enable = true;

  services.userborn.enable = true;

  systemd.network.enable = true;
  networking.useDHCP = false;

  systemd.network.networks."10-enp3s0" = {
    matchConfig.Name = "enp3s0";
    networkConfig = {
      DHCP = "ipv4";
      IPv6AcceptRA = true;
    };
    linkConfig.RequiredForOnline = "routable";
  };

  fileSystems."/persistent" = {
    device = "/dev/disk/by-id/virtio-PERSISTENT";
    fsType = "ext4";
    neededForBoot = true;
    autoFormat = true;
    autoResize = true;
  };

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
      growpart /dev/disk/by-id/virtio-NIXSTORE 2 || true;
      resize2fs /dev/disk/by-id/virtio-NIXSTORE-part2
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
