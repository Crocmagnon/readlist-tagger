{
  description = "Auto-tag unread Wallabag articles by theme with Claude";

  inputs.nixpkgs.url = "nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "x86_64-darwin"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        rec {
          # go.mod requires Go 1.27.
          wallabag-tagger = (pkgs.buildGoModule.override { go = pkgs.go_1_27; }) {
            pname = "wallabag-tagger";
            version = "latest";
            src = ./.;
            vendorHash = null; # no dependencies
            subPackages = [ "cmd/wallabag-tagger" ];
          };
          default = wallabag-tagger;
        }
      );
    };
}
