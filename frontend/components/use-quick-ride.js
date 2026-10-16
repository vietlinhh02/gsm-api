"use client";

import { useEffect, useState } from "react";
import { followOrder } from "#lib/order-stream.mjs";
import { requestApi, apiErrorMessage } from "#lib/http.mjs";
import {
  presetPoints,
  selectedPoint,
  selectedBooking,
  tripRequest,
  swapPoints,
} from "#lib/quick-ride.mjs";

async function accessToken(state) {
  if (state.token) return state.token;
  const result = await requestApi({
    path: "/v1/sandbox/token",
    method: "POST",
    contentType: "application/x-www-form-urlencoded",
    body: new URLSearchParams({
      client_id: state.clientId,
      client_secret: state.clientSecret,
      grant_type: "client_credentials",
      scope: "sandbox.ride.trips",
    }),
  });
  return tokenFromResult(result);
}

function tokenFromResult(result) {
  if (result.status !== 200 || !result.data?.access_token) {
    throw new Error(apiErrorMessage(result));
  }
  return result.data.access_token;
}

function apiCaller(token) {
  const prefix = "/v1/sandbox";
  return async (path, body) => {
    const result = await requestApi({
      path: prefix + path,
      method: body === undefined ? "GET" : "POST",
      token,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (result.status < 200 || result.status >= 300) {
      const failure = new Error(`HTTP ${result.status}: ${apiErrorMessage(result)}`);
      failure.response = result;
      throw failure;
    }
    return result;
  };
}

async function comparePrices(call, form) {
  const catalog = await call("/ride/list-available-services", {
    pickups: [form.points[0]],
    dropoffs: [form.points[1]],
    service_types: ["RIDE-TRIP"],
  });
  const services = catalog.data.data
    .map((item) => item.service)
    .filter((service) => ["Car", "Motorcycle"].includes(service.travel_mode));
  const request = tripRequest(form, services);
  const result = await call("/ride/estimate/order", request);
  return { request, result };
}

export function useQuickRide(model) {
  const { state, patch } = model;
  const [form, setForm] = useState({
    points: presetPoints("center"),
    name: "Nguyễn Văn A",
    phone: "+84900000000",
    payment: "cash",
    serviceId: 2179,
    target: 0,
    query: "",
    preset: "center",
  });
  const [quote, setQuote] = useState(null);
  const [order, setOrder] = useState(null);
  const [places, setPlaces] = useState([]);
  const [error, setError] = useState("");

  useSandboxOrderStream(state, order, setOrder, setError);

  async function execute(task) {
    if (state.busy) return;
    patch({ busy: true, error: "", booking: null });
    setError("");
    try {
      const token = await accessToken(state);
      patch({ token });
      const result = await task(apiCaller(token));
      patch({ result, busy: false });
    } catch (failure) {
      setError(failure.message ?? "Không hoàn tất yêu cầu. Thử lại.");
      patch({ busy: false, result: failure.response ?? null });
    }
  }

  function updateForm(values) {
    setForm((previous) => ({ ...previous, ...values }));
    if (["points", "name", "phone", "payment"].some((key) => Object.hasOwn(values, key))) {
      setQuote(null);
    }
    if (Object.hasOwn(values, "target") || Object.hasOwn(values, "query")) setPlaces([]);
  }

  function chooseLocation(location) {
    const points = form.points.map((point, index) =>
      index === form.target ? selectedPoint(point, location) : point,
    );
    updateForm({ points, preset: "custom" });
    setPlaces([]);
  }

  const actions = rideActions({ model, form, quote, order, setQuote, setOrder, execute });
  return {
    form,
    quote,
    order,
    places,
    error,
    updateForm,
    chooseLocation,
    ...actions,
    swap: () => updateForm({ points: swapPoints(form.points), preset: "custom" }),
    preset: (id) => updateForm({ points: presetPoints(id), preset: id }),
    search: () =>
      execute(async (call) => {
        if (!form.query.trim()) throw new Error("Nhập địa chỉ rồi bấm Tìm địa chỉ.");
        const query = encodeURIComponent(form.query.trim());
        const result = await call(`/place/geocode?text=${query}&max=5`);
        setPlaces(result.data.data);
        return result;
      }),
  };
}

function rideActions({ model, form, quote, order, setQuote, setOrder, execute }) {
  const { patch } = model;
  const detailsPath = (id) => `/ride/orders/${encodeURIComponent(id)}?extra_components=driver,path`;
  return {
    compare: () =>
      execute(async (call) => {
        setQuote(null);
        const next = await comparePrices(call, form);
        setQuote(next);
        return next.result;
      }),
    book: () =>
      execute(async (call) => {
        if (!quote) throw new Error("So sánh giá trước khi đặt chuyến.");
        const booking = selectedBooking(quote.request, quote.result.data, form.serviceId);
        const created = await call("/ride/create-order", booking);
        setOrder(created.data.data);
        setQuote(null);
        patch({ orderId: created.data.data.id });
        return created;
      }),
    refresh: () =>
      execute(async (call) => {
        const result = await call(detailsPath(order.id));
        setOrder(result.data.data);
        return result;
      }),
    simulate: (status) =>
      execute(async (call) => {
        await call("/ride/update-status", { update_type: "order", order_id: order.id, status });
        const result = await call(detailsPath(order.id));
        setOrder(result.data.data);
        return result;
      }),
    cancel: () =>
      execute(async (call) => {
        await call("/ride/cancel-order", {
          order_id: order.id,
          cancel_code: 9000,
          cancel_message: "Khách hủy chuyến",
        });
        const result = await call(detailsPath(order.id));
        setOrder(result.data.data);
        return result;
      }),
  };
}

function useSandboxOrderStream(state, order, setOrder, setError) {
  const { token } = state;
  const id = order?.id;
  const terminal = ["COMPLETED", "CANCELLED"].includes(order?.status);
  useEffect(() => {
    if (!token || !id || terminal) return;
    const controller = new AbortController();
    void followOrder({
      id,
      token,
      signal: controller.signal,
      onOrder: (next) => {
        setOrder(next);
        setError("");
      },
      onError: (message) => setError(message),
    });
    return () => controller.abort();
  }, [token, id, terminal, setOrder, setError]);
}
