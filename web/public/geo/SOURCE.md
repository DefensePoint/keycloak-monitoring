# Vendored world basemap data

`countries-50m.json` is the world basemap KMT draws its maps on. It is
vendored deliberately: the platform must not call any third-party service at
runtime, because deployments can be airgapped.

## Provenance

- File: `countries-50m.json`, TopoJSON, 241 country polygons plus a merged
  land outline.
- Taken verbatim from the `world-atlas` npm package, version 2.0.2
  (https://github.com/topojson/world-atlas), file `countries-50m.json`.
- Derived from Natural Earth's 1:50m Admin 0 Countries dataset
  (https://www.naturalearthdata.com).

## Integrity

    sha256  04342cdc1e3016bcd7db1630de95684d67b79fe3c8c460321e87aef469502394
    bytes   756420

Recorded because nobody reviews a 756 KB JSON blob in a diff. Verify after
re-vendoring, and on any change to this file that is not accompanied by a
version bump above:

    shasum -a 256 web/public/geo/countries-50m.json

A mismatch means the file is not the one this document describes. Note that
corrupt or substituted geometry fails quietly: the map still renders, just
with the wrong coastlines, so nothing else will raise the alarm.

## `cities-10m.json`

City labels for the basemap. Country outlines alone leave an inland marker
sitting in an unlabelled polygon, so these are what let a reader place a point.

- A slimmed extract of Natural Earth's 1:10m Populated Places
  (`ne_10m_populated_places_simple.geojson`, 4.8 MB), reduced to the fields the
  maps use: `name`, `country`, `pop`, `minZoom`, `capital`, `lat`, `lon`.
- 7,342 cities. 783876 bytes raw, about 161 KB gzipped.
- 1:10m rather than the 1:50m tier used for the boundaries, because 1:50m is
  too thin once a map is zoomed into a single country: 7 cities for the United
  Kingdom, 5 for Germany, 1 for Albania, against 57, 58 and 26 here.
- Sorted by `minZoom` ascending, then population descending, so a renderer that
  truncates the list keeps the most significant cities.
- `minZoom` is Natural Earth's own `min_zoom` label guidance, rounded to one
  decimal. Tokyo is 1.7, a French regional capital around 6. Thinning labels by
  it follows cartographic judgement rather than an arbitrary population cutoff.

Integrity:

    sha256  382831018e7a022fec23b6a672c350dab485d9e02b4b6e38ae1bcd92d45e58dd
    bytes   783876

Regenerate with:

    curl -sL -o places.geojson \
      https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_10m_populated_places_simple.geojson

then map each feature to the fields above, sort, and re-record the checksum.

## Licence

Natural Earth data is in the **public domain**. Their terms state no permission
is needed and no attribution is required, though crediting Natural Earth is
encouraged, which is why the maps name it in the attribution control.

The `world-atlas` packaging scripts are ISC licensed (Copyright 2013-2019
Michael Bostock). That covers the build tooling, not the data in this file.

## Why 1:50m

1:110m (108 KB) is too coarse once a map is zoomed to a country, and 1:10m
(3.7 MB) buys detail that IP geolocation cannot justify. 1:50m is 756 KB raw
and about 231 KB gzipped, and it is served as a static asset rather than
bundled into the JavaScript, so it is fetched once and cached.

The maps cap zoom accordingly. Past roughly zoom 7 these boundaries turn
visibly angular, and there is nothing behind them to reveal, so the cap is
honest rather than restrictive.

## Updating

Bump the `world-atlas` version, re-extract the same filename, then update both
the version and the checksum recorded above:

    npm pack world-atlas@<version>
    tar -xzOf world-atlas-<version>.tgz package/countries-50m.json \
      > web/public/geo/countries-50m.json
    shasum -a 256 web/public/geo/countries-50m.json

## Adding more reference data

More vendored geography is fine and follows this same pattern: download once
during development, commit it, serve it from our own origin. Natural Earth's
populated places and admin-1 boundaries are the obvious candidates, and being
public domain they carry no attribution obligation. Prefer them over the
GeoNames-derived npm packages, which are CC-BY and would attach a requirement
to a product we ship to customers.

One distinction to hold on to, because the two sound alike and are not:

- **Vendored reference data** is safe. The file ships with the application and
  the browser reads it from our own origin.
- **Geocoding at runtime** is not. Turning an address or an IP into
  coordinates by calling a lookup service is a third-party request on the
  request path, which is prohibited and breaks airgapped deployments.

What makes the maps work offline is that the lookup already happened upstream
in AMFA. The coordinates arrive with the event; the map only draws them.
