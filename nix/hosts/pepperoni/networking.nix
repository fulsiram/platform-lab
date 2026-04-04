{ lib, ... }:
{
  networking = {
    useDHCP = false;
    hostName = "hostname";

    interfaces.enp7s0 = {
      ipv4.addresses = [
        {
          address = "162.55.238.111";
          prefixLength = 32;
        }
      ];
    };

    defaultGateway = {
      address = "162.55.238.65";
      interface = "enp7s0";
    };

    nameservers = [
      "1.1.1.1"
      "8.8.8.8"
    ];
  };

  networking.nftables.enable = true;
  networking.firewall = {
    enable = true;
    allowedTCPPorts = lib.mkForce [
      22
      80
      443
      6443
    ];
  };
}
