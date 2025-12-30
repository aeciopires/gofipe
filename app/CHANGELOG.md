# Changelog

<!-- TOC -->

- [Changelog](#changelog)
- [v2.0.0](#v200)
- [v1.0.0](#v100)

<!-- TOC -->

# v2.0.0

Date: 12/30/2025

- Split frontend assets into `static/css/style.css` and `static/js/app.js` (clean separation of concerns).
- Implemented backend in-memory caching for brands, models and years to reduce repeated external FIPE requests.
- Added `/api/priceHistory` endpoint and frontend history UI to display price history (default 12 months).
- Added concurrent fetching helper and used parallel requests where appropriate.
- Improved error handling for external requests and clearer HTTP status codes.
- Added Prometheus metrics: `fipe_price_min`, `fipe_price_max`, `fipe_fuel_count`, `fipe_brand_search_count`.
- Updated Grafana dashboard (`app/dashboard/dash-grafana.json`) to include new metrics panels.
- Added day/night theme support and modernized colors and layout.
- Updated `README.md` to document new features.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:2.0.0 (alpine 3.23.2) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘

# v1.0.0

Date: 12/29/2025

- Initial version of the application.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:1.0.0 (alpine 3.23.2) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘