{ lib, ... }:
{
  networking = {
    useDHCP = false;
    hostName = "pepperoni";

    interfaces.enp7s0 = {
      ipv4.addresses = [
        {
          address = "162.55.238.111";
          prefixLength = 32;
        }
      ];
      ipv6.addresses = [
        {
          address = "2a01:4f8:271:3fce::1";
          prefixLength = 64;
        }
      ];
    };

    defaultGateway = {
      address = "162.55.238.65";
      interface = "enp7s0";
    };

    defaultGateway6 = {
      address = "fe80::1";
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

  networking.firewall.extraInputRules = ''
    iifname "cni0" tcp dport 10250 accept
  '';
}
