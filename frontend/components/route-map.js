"use client";

import { useEffect, useRef, useState } from "react";
import {
  Map as VectorMap,
  Marker,
  NavigationControl,
  LngLatBounds,
  setWorkerUrl,
} from "maplibre-gl";
import maplibrePackage from "maplibre-gl/package.json";
import { Banner } from "@astryxdesign/core/Banner";
import { VStack } from "@astryxdesign/core/Stack";

const mapStyles = { light: "positron", dark: "dark", standard: "liberty" };
const routeSource = "ride-route";

function drawRoute(instance, geometry, host) {
  if (!instance.isStyleLoaded()) return;
  const features = geometry ? [{ type: "Feature", properties: {}, geometry }] : [];
  const data = { type: "FeatureCollection", features };
  const source = instance.getSource(routeSource);
  if (source) {
    source.setData(data);
    return;
  }
  const style = getComputedStyle(host);
  instance.addSource(routeSource, { type: "geojson", data });
  for (const [id, color, width] of [
    ["ride-outline", style.getPropertyValue("--map-route-outline").trim(), 8],
    ["ride-line", style.getPropertyValue("--map-route-color").trim(), 5],
  ]) {
    instance.addLayer({
      id,
      type: "line",
      source: routeSource,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": color, "line-width": width },
    });
  }
}

function fitRoute(instance, points, geometry, host) {
  if (host.clientWidth === 0) return;
  const coordinates = geometry?.coordinates ?? points.map((p) => [p.longitude, p.latitude]);
  const bounds = new LngLatBounds();
  for (const coordinate of coordinates) bounds.extend(coordinate);
  if (!bounds.isEmpty()) instance.fitBounds(bounds, { padding: 56, maxZoom: 16, duration: 0 });
}

function pointMarker(instance, point, index) {
  const element = document.createElement("button");
  element.type = "button";
  element.className = `map-pin ${index === 0 ? "map-pin-pickup" : "map-pin-dropoff"}`;
  const label = `${index === 0 ? "Điểm đón" : "Điểm đến"}: ${point.address}`;
  element.setAttribute("aria-label", label);
  element.title = label;
  const icon = document.createElement("i");
  icon.className = "map-pin-icon";
  icon.setAttribute("aria-hidden", "true");
  element.append(icon);
  element.addEventListener("click", (event) => event.stopPropagation());
  return new Marker({ element, anchor: "bottom" })
    .setLngLat([point.longitude, point.latitude])
    .addTo(instance);
}

function createMap(host, appearance) {
  setWorkerUrl(`/maplibre/${maplibrePackage.version}/maplibre-gl-worker.mjs`);
  const instance = new VectorMap({
    container: host,
    center: [105.835, 21.03],
    zoom: 13,
    style: `https://tiles.openfreemap.org/styles/${mapStyles[appearance] ?? "positron"}`,
    attributionControl: { compact: false },
    locale: {
      "NavigationControl.ZoomIn": "Phóng to",
      "NavigationControl.ZoomOut": "Thu nhỏ",
      "NavigationControl.ResetBearing": "Đưa bản đồ về hướng Bắc",
      "AttributionControl.ToggleAttribution": "Nguồn dữ liệu bản đồ",
    },
  });
  instance.addControl(new NavigationControl({ showCompass: false }), "bottom-right");
  instance.getCanvas().setAttribute("aria-label", "Bản đồ hành trình");
  return instance;
}

function mountMap({ host, appearance, current, map, markers, setError }) {
  let active = true;
  const instance = createMap(host.current, appearance);
  map.current = instance;
  instance.on("click", ({ lngLat }) => {
    if (!current.current.isDisabled) {
      current.current.onSelect({ latitude: lngLat.lat, longitude: lngLat.lng });
    }
  });
  instance.on("error", () => {
    if (active) setError("Chưa tải được bản đồ. Kiểm tra kết nối mạng.");
  });
  instance.on("load", () => {
    if (!active) return;
    setError("");
    const value = current.current;
    drawRoute(instance, value.geometry, host.current);
    fitRoute(instance, value.points, value.geometry, host.current);
  });
  markers.current = current.current.points.map((point, index) =>
    pointMarker(instance, point, index),
  );
  const observer = new ResizeObserver(() => {
    if (!active || host.current.clientWidth === 0) return;
    instance.resize();
    fitRoute(instance, current.current.points, current.current.geometry, host.current);
  });
  observer.observe(host.current);
  return () => {
    active = false;
    observer.disconnect();
    for (const marker of markers.current) marker.remove();
    instance.remove();
    map.current = null;
  };
}

export default function RouteMap({ points, geometry, onSelect, isDisabled, appearance }) {
  const host = useRef(null);
  const map = useRef(null);
  const markers = useRef([]);
  const current = useRef({ points, geometry, onSelect, isDisabled });
  const [error, setError] = useState("");
  useEffect(() => {
    current.current = { points, geometry, onSelect, isDisabled };
  }, [points, geometry, onSelect, isDisabled]);

  useEffect(() => {
    let disposed = false;
    try {
      return mountMap({ host, appearance, current, map, markers, setError });
    } catch (failure) {
      queueMicrotask(() => {
        if (!disposed) {
          setError("Không mở được bản đồ. Hãy bật tăng tốc phần cứng trong trình duyệt.");
          console.error("Không khởi tạo được bản đồ", failure);
        }
      });
      return () => {
        disposed = true;
      };
    }
  }, [appearance]);

  useEffect(() => {
    if (!map.current) return;
    for (const marker of markers.current) marker.remove();
    markers.current = points.map((point, index) => pointMarker(map.current, point, index));
    drawRoute(map.current, geometry, host.current);
    fitRoute(map.current, points, geometry, host.current);
  }, [points, geometry]);

  return (
    <VStack gap={3}>
      <section
        ref={host}
        className={`route-map map-style-${appearance}`}
        aria-label="Bản đồ hành trình"
      />
      {error && <Banner status="warning" title="Bản đồ chưa sẵn sàng" description={error} />}
    </VStack>
  );
}
