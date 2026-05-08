{
  disko.imageBuilder.imageFormat = "qcow2";

  disko.devices.disk = {
    root = {
      type = "disk";
      device = "/dev/disk/by-label/nixos";
      imageSize = "1G";
      content = {
        type = "gpt";
        partitions = {
          ESP = {
            size = "512M";
            type = "EF00";
            content = {
              type = "filesystem";
              format = "vfat";
              mountpoint = "/boot";
              mountOptions = [ "umask=0077" ];
            };
          };
          root = {
            size = "100%";
            content = {
              type = "filesystem";
              format = "ext4";
              mountpoint = "/";
              extraArgs = [
                "-L"
                "nixos"
              ];
            };
          };
        };
      };
    };

    nix = {
      type = "disk";
      device = "/dev/disk/by-id/virtio-NIXSTORE";
      imageSize = "3G";
      content = {
        type = "gpt";
        partitions = {
          nix = {
            size = "100%";
            content = {
              type = "filesystem";
              format = "ext4";
              mountpoint = "/nix";
              extraArgs = [
                "-L"
                "nixstore"
              ];
              mountOptions = [ "noatime" ];
            };
          };
        };
      };
    };
  };
}
