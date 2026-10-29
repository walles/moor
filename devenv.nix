{ pkgs, lib, ... }:

{
  languages.go.enable = true;

  env.GOCACHE = "/tmp/go-cache";
  env.GOPATH = lib.mkForce "/tmp/go-path";

  packages = [
    pkgs.golangci-lint
    pkgs.mandoc
  ];
}
