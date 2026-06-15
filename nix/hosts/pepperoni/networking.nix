{ lib, ... }:
let
  enp7s0Network = {
    matchConfig.Name = "enp7s0";

    address = [
      "162.55.238.111/32"
      "2a01:4f8:271:3fce::1/128"
    ];
    routes = [
      {
        Gateway = "162.55.238.65";
        GatewayOnLink = true;
      }
      {
        Gateway = "fe80::1";
      }
    ];

    linkConfig.RequiredForOnline = "routable";
  };
in
{
  networking.hostName = "pepperoni";
  networking.nameservers = [
    "1.1.1.1"
    "8.8.8.8"
  ];

  networking.useDHCP = false;
  systemd.network.enable = true;
  systemd.network.networks."10-enp7s0" = enp7s0Network;
  boot.initrd.systemd.network.networks."10-enp7s0" = enp7s0Network;

  networking.nftables.enable = true;
  networking.firewall = {
    enable = true;
    allowedTCPPorts = lib.mkForce [
      22
      80
      443
      6443
    ];
    checkReversePath = false;
  };

  networking.firewall.trustedInterfaces = [
    "cilium_host"
    "cilium_net"
    "lxc*"
  ];
}
