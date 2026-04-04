{
  disko.devices = {
    disk = {
      # Devices will be mounted and formatted in alphabetical order, and btrfs can only mount raids
      # when all devices are present. So we define an "empty" luks device on the first disk,
      # and the actual btrfs raid on the second disk, and the name of these entries matters!
      disk1 = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-SAMSUNG_MZQLB1T9HAJR-00007_S439NC0R700843";
        content = {
          type = "gpt";
          partitions = {
            boot = {
              size = "1M";
              type = "EF02"; # BIOS boot partition
            };
            ESP = {
              size = "1G";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot0";
                mountOptions = [ "umask=0077" ];
              };
            };
            crypt_p1 = {
              label = "crypt_p1";
              size = "100%";
              content = {
                type = "luks";
                name = "p1"; # device-mapper name when decrypted
                settings = {
                  allowDiscards = true;
                };
              };
            };
          };
        };
      };
      disk2 = {
        type = "disk";
        device = "/dev/disk/by-id/nvme-SAMSUNG_MZQLB1T9HAJR-00007_S439NC0R700977";
        content = {
          type = "gpt";
          partitions = {
            boot = {
              size = "1M";
              type = "EF02"; # BIOS boot partition
            };
            ESP = {
              size = "1G";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot1";
                mountOptions = [ "umask=0077" ];
              };
            };
            crypt_p2 = {
              label = "crypt_p2";
              size = "100%";
              content = {
                type = "luks";
                name = "p2";
                # Remove settings.keyFile if you want to use interactive password entry
                settings = {
                  allowDiscards = true;
                };
                content = {
                  type = "btrfs";
                  extraArgs = [
                    "-d raid1"
                    "/dev/mapper/p1" # Use decrypted mapped device, same name as defined in disk1
                  ];
                  subvolumes = {
                    "/root" = {
                      mountpoint = "/";
                      mountOptions = [
                        "compress=zstd"
                        "noatime"
                        "ssd"
                      ];
                    };
                    "/home" = {
                      mountpoint = "/home";
                      mountOptions = [
                        "compress=zstd"
                        "noatime"
                        "ssd"
                      ];
                    };
                    "/nix" = {
                      mountpoint = "/nix";
                      mountOptions = [
                        "compress=zstd"
                        "noatime"
                        "ssd"
                      ];
                    };
                  };
                };
              };
            };
          };
        };
      };
    };
  };
}
