{
  description = "Auto-tag unread read-it-later articles (Readeck, Wallabag) by theme with Claude";

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
          # go.mod requires Go 1.27.
          tagger =
            name:
            (pkgs.buildGoModule.override { go = pkgs.go_1_27; }) {
              pname = name;
              version = "latest";
              src = ./.;
              vendorHash = null; # no dependencies
              subPackages = [ "cmd/${name}" ];
            };
        in
        rec {
          readeck-tagger = tagger "readeck-tagger";
          wallabag-tagger = tagger "wallabag-tagger";
          default = readeck-tagger;
        }
      );
    };
}
