export const hanoiPresets = [
  {
    id: "center",
    label: "Hồ Hoàn Kiếm → Lotte Center",
    locations: [
      [21.0287, 105.8524, "Hồ Hoàn Kiếm, Hà Nội"],
      [21.0324, 105.812, "Lotte Center, 54 Liễu Giai, Hà Nội"],
    ],
  },
  {
    id: "airport",
    label: "Hồ Hoàn Kiếm → Sân bay Nội Bài",
    locations: [
      [21.0287, 105.8524, "Hồ Hoàn Kiếm, Hà Nội"],
      [21.2142, 105.8028, "Nhà ga T1, Sân bay Nội Bài, Hà Nội"],
    ],
  },
  {
    id: "short",
    label: "Nhà hát Lớn → Hồ Hoàn Kiếm",
    locations: [
      [21.0242, 105.8574, "Nhà hát Lớn Hà Nội"],
      [21.0287, 105.8524, "Hồ Hoàn Kiếm, Hà Nội"],
    ],
  },
];

export const money = (value) => new Intl.NumberFormat("vi-VN").format(value) + "đ";

/** Build complete booking points from a local Hanoi route preset. */
export function presetPoints(id) {
  const preset = hanoiPresets.find((item) => item.id === id) ?? hanoiPresets[0];
  return preset.locations.map(([latitude, longitude, address], index) => ({
    type: index === 0 ? "PICK UP" : "DROP OFF",
    latitude,
    longitude,
    address,
    short_address: address,
    name: "Nguyễn Văn A",
    phone_number: "+84900000000",
  }));
}

/** Keep passenger details when selecting a point; replace stale place identifiers. */
export function selectedPoint(previous, location) {
  const point = { ...previous };
  delete point.place_id;
  const address =
    location.label ?? `${location.latitude.toFixed(6)}, ${location.longitude.toFixed(6)}`;
  return {
    ...point,
    latitude: location.latitude,
    longitude: location.longitude,
    address: address.slice(0, 512),
    short_address: address.slice(0, 512),
    ...(location.place_id ? { place_id: location.place_id } : {}),
  };
}

export function swapPoints(points) {
  return points.toReversed().map((point, index) => ({
    ...point,
    type: index === 0 ? "PICK UP" : "DROP OFF",
  }));
}

/** Validate passenger input before obtaining a quote or creating a local order. */
export function tripRequest(form, services) {
  if (!form.name.trim() || form.name.length > 256) {
    throw new Error("Nhập tên khách từ 1 đến 256 ký tự.");
  }
  if (!/^\+[1-9]\d{7,14}$/.test(form.phone)) {
    throw new Error("Số điện thoại cần định dạng quốc tế, ví dụ +84900000000.");
  }
  if (services.length === 0) throw new Error("Không có dịch vụ ô tô/xe máy trong catalog.");
  return {
    service_ids: services.map((service) => ({ id: service.id })),
    order_time: 0,
    payment_method: form.payment,
    points: form.points.map((point) => ({
      ...point,
      name: form.name.trim(),
      phone_number: form.phone,
    })),
  };
}

/** Bind the exact selected service and quote to the request sent for estimation. */
export function selectedBooking(request, response, serviceId) {
  const item = quoteEstimates(response).find(
    (estimate) => estimate.service.id === serviceId && !estimate.fee_unavailable,
  );
  if (!item?.estimate_info) {
    throw new Error("Dịch vụ chưa có báo giá hợp lệ. Hãy so sánh giá lại.");
  }
  const info = item.estimate_info;
  const time = Number(response.data.order_time);
  if (!info.fee_id || !Number.isSafeInteger(time) || time <= 0) {
    throw new Error("Dịch vụ chưa có báo giá hợp lệ. Hãy so sánh giá lại.");
  }
  if (!Number.isFinite(info.expires_at) || info.expires_at * 1000 <= Date.now()) {
    throw new Error("Báo giá đã hết hạn. Hãy so sánh giá lại trước khi đặt.");
  }
  return {
    service_id: serviceId,
    service_type: "RIDE-TRIP",
    fee_id: info.fee_id,
    order_time: time,
    payment_method: request.payment_method,
    points: request.points,
  };
}

export function quoteEstimates(response) {
  return response?.data?.service_estimates ?? [];
}

export function quoteGeometry(quote) {
  if (!quote) return undefined;
  const item = quoteEstimates(quote.result.data).find((estimate) => !estimate.fee_unavailable);
  return item?.estimate_info?.trip?.geometry;
}

export function simulationActions(status) {
  return {
    assign: status === "FINDING",
    start: status === "ASSIGNED",
    complete: status === "IN PROCESS",
    cancel: ["FINDING", "ASSIGNED", "SCHEDULING", "WAITING_FOR_PAYMENT"].includes(status),
  };
}
