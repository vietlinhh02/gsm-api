# GreenSM Rides API simulator

A standalone Go implementation of the Vietnam Rides API contract. It stores simulated orders in PostgreSQL and uses OpenStreetMap-backed geocoding and routing for address lookup and route estimates. It does not create real GreenSM orders or reproduce GSM's live pricing/dispatch decisions.

## Run

Requirements: Go 1.27+, Node.js 20.9+, pnpm, Docker Compose.

Start PostgreSQL and the Go API in one terminal:

```sh
cp .env.example .env
docker compose up -d --wait
set -a
. ./.env
set +a
PORT=8081 go run ./cmd/server
```

Start the Next.js developer guide in another terminal:

```sh
cd frontend
pnpm install --frozen-lockfile
pnpm dev
```

Open `http://localhost:3000`. The guide includes endpoint examples and a playground. Its `/api/backend` route proxies requests to `API_ORIGIN` (defaults to `http://localhost:8081`) to avoid browser CORS errors. Schema migrations and local service seeds run when the Go server starts. Local credentials default to `local-client` / `local-secret`; replace them outside local development.

`pnpm dev` and `pnpm build` copy the installed MapLibre worker into a versioned folder under
`public/maplibre/`. The map loads this worker from the frontend's own origin. Pickup and
destination markers use local [Phosphor SVG icons](https://phosphoricons.com/).

## Deployment

The backend deploys through GHCR into Coolify; the frontend is deployed with Vercel CLI.
Open the [hosted guide and playground](https://gsm-api-eight.vercel.app). See
[deployment instructions](deploy/README.md) for runtime configuration, credentials and operations.

## Authentication

Get a Sandbox token:

```sh
curl -X POST http://localhost:8081/v1/sandbox/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'client_id=local-client' \
  --data-urlencode 'client_secret=local-secret' \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'scope=sandbox.ride.trips'
```

Pass the returned token as `Authorization: Bearer <token>`.

## Implemented routes

Only Sandbox is exposed under `/v1/sandbox`: token exchange at `/token`, ride APIs under `/ride`, geocoding at `/place/geocode`, and simulated transitions at `/ride/update-status`. Production routes are not registered.

`GET /list-user-orders` returns this client's stored orders. The supplied docs did not define its filter parameter names, so no undocumented filters are implemented. Service catalog is local seed data. RIDE-HOUR remains a demo at 175,000 VND/hour, less the supplied discount.

## Hanoi playground and simulated fares

Open `http://localhost:3000/playground`; Sandbox is the only test environment.
Select a Hanoi route preset, click the OpenStreetMap map to choose pickup/dropoff, or explicitly
search an address. Compare car/bike fares, choose a vehicle, then book. Tokens are obtained
automatically using the editable local credentials. Sandbox trips automatically assign a driver, simulate pickup/dropoff, start and complete.
Each step takes about five seconds (roughly 55 seconds for a two-point trip).
The playground subscribes to an authenticated SSE stream for active sandbox orders.
It receives an initial snapshot and updates after committed order/point changes, without polling. Manual status, cancellation
and refresh buttons remain available for testing. Scheduled trips begin after `order_time`. The JSON endpoint tester remains available below.

Trip prices use the [operator's Hanoi reference table from March 2025](https://www.xanhsm.com/news/du-lich-sinh-thai-ban-rom-soc-son-ha-noi), inspected on 2026-10-15:

- Car: 30,500 VND for the first 2 km; 15,500 VND/km above 2 through 12 km;
  14,500 VND/km above 12 through 25 km; 12,500 VND/km above 25 km.
- Motorcycle: 13,200 VND for the first 2 km; 4,200 VND/km above 2 km;
  plus 275 VND/minute over the whole estimated journey.

Fractional km/minutes are prorated; each component rounds to the nearest VND and the components
sum to the fare. The table does not disclose app rounding, so this rounding is a simulator choice.
`partner_pay` has no additional payment surcharge. Optional car add-ons retain their local demo
charges (10,000 VND for ID 10; 5,000 VND for IDs 12–14), separate from the reference tariff.
The quote response includes its dated source and component calculations in `estimate_info.pricing`,
and GeoJSON route geometry in `estimate_info.trip.geometry`. Created order details preserve the
accepted quote in `fare`, so a later tariff change does not change that order's price.

These are dated reference prices, not current guaranteed app quotes. The simulator does not
calculate live supply/demand, traffic, waiting charges, tolls, insurance, or promotions.
OSRM's driving profile is also used to approximate motorcycle routes and duration; it does not
model motorcycle-specific access restrictions. Orders are persisted, but drivers are fixtures,
assigned by the automatic sandbox worker or manual sandbox status actions. There is no live dispatch, GPS tracking or actual
payment processing. Service area metadata is Hanoi; it does not enforce a geographic boundary.

## Live sandbox order updates

`GET /v1/sandbox/ride/orders/{id}/events` returns `text/event-stream` and requires the
same sandbox Bearer token as order details. Each `event: order` contains `data: {"data": ...}`
with the current order, driver, vehicle and path. Only the owning client can subscribe.
The stream closes after `COMPLETED` or `CANCELLED`. Idle streams send a comment heartbeat
every 15 seconds, without querying the order again. The browser reconnects with exponential
backoff after a disconnect and receives the latest snapshot; intermediate changes during a
disconnect are not replayed. No token is placed in the URL.

The Next.js proxy forwards the response body as a stream and disables proxy buffering.
Its connection timeout applies only until upstream headers arrive for this endpoint.
Subscriptions currently live in one API process: run one API instance with its sandbox worker.
Multiple API replicas would need a shared notification channel before using this stream.
Outbound webhooks remain independent of SSE.

## OSM and webhooks

Geocoding uses Nominatim and routing uses OSRM, configured with `NOMINATIM_URL` and `OSRM_URL`.
If the default public Nominatim instance fails, the API retries geocoding through the
[Photon public demo](https://github.com/komoot/photon), also backed by OpenStreetMap.
Custom Nominatim instances do not forward queries to Photon. A successful empty Nominatim
result does not trigger failover. Provider errors remain visible when both services fail.
The default Nominatim service limits usage to one request per second, disallows client-side
autocomplete, and requires an identifying User-Agent and attribution. Search is button-triggered
and successful results are cached. Photon permits reasonable demo usage and has no availability
guarantee. Use an appropriate hosted provider or self-hosted OSM services for production traffic.

New sandbox orders run automatically even without a webhook URL. The worker persists its
progress in PostgreSQL and resumes pending trips after restart. Cancelled and completed trips
stop advancing. Orders created before this feature remain manual. Status updates and webhook
events commit atomically.

Set `WEBHOOK_URL` to your HTTP callback receiver to enable real outbound callbacks.
An empty URL disables callback delivery; no receiver URL is fabricated. Set `WEBHOOK_SIGNING_KEY` to add `X-VSF-Signature` (HMAC-SHA256 over the raw JSON body). Deliveries are persisted and retried up to seven times; delivery is asynchronous. Sandbox does not emit `driver.location.updated` events.

## Verification

```sh
go test ./...
TEST_DATABASE_URL='postgres://gsm_api:gsm_api_local@localhost:55432/gsm_api?sslmode=disable' go test ./internal/api -run TestRideAPIEndToEnd -count=1
```

The PostgreSQL-backed end-to-end test is skipped unless `TEST_DATABASE_URL` is set. Start Compose before running it; use a disposable test database because migrations and simulated orders are written to it.

Frontend quality checks run from `frontend/` with `pnpm lint`, `pnpm build`, and `pnpm audit --audit-level=moderate`.
