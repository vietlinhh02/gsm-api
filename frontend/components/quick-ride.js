"use client";

import dynamic from "next/dynamic";
import { useState } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Divider } from "@astryxdesign/core/Divider";
import { Grid } from "@astryxdesign/core/Grid";
import { Link } from "@astryxdesign/core/Link";
import { Selector } from "@astryxdesign/core/Selector";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Table } from "@astryxdesign/core/Table";
import { Heading, Text } from "@astryxdesign/core/Text";
import { TextInput } from "@astryxdesign/core/TextInput";
import {
  hanoiPresets,
  money,
  simulationActions,
  quoteEstimates,
  quoteGeometry,
} from "#lib/quick-ride.mjs";
import { useQuickRide } from "@/components/use-quick-ride";
import { useCurrentLocation } from "@/components/use-current-location";
import SiteIcon from "@/components/site-icon";

const RouteMap = dynamic(() => import("@/components/route-map"), {
  ssr: false,
  loading: () => <Text as="p">Đang tải bản đồ…</Text>,
});

const orderStatusNames = {
  FINDING: "Đang tìm tài xế",
  ASSIGNED: "Đã có tài xế",
  "IN PROCESS": "Đang di chuyển",
  COMPLETED: "Đã hoàn thành",
  CANCELLED: "Đã hủy",
  SCHEDULING: "Đã lên lịch",
  WAITING_FOR_PAYMENT: "Chờ thanh toán",
};

function fareLineRow(item) {
  const quantity = item.quantity.toLocaleString("vi-VN", { maximumFractionDigits: 3 });
  return {
    ...item,
    calculation: `${quantity} ${item.unit} × ${money(item.rate)}`,
    amount: money(item.amount),
  };
}

function routeSummary(trip) {
  const distance = trip.distance_estimate.toFixed(2);
  const minutes = (trip.duration_estimate / 60).toFixed(1);
  return `${distance} km · ${minutes} phút`;
}

function PointControls({ ride, busy, geometry }) {
  const { form, updateForm } = ride;
  const location = useCurrentLocation(ride.chooseLocation);
  const [appearance, setAppearance] = useState("light");
  const disabled = busy || location.busy;
  return (
    <VStack gap={4}>
      <Selector
        label="Hành trình gợi ý"
        startIcon={<SiteIcon name="navigation-arrow" />}
        value={form.preset}
        options={[
          ...hanoiPresets.map((item) => ({ value: item.id, label: item.label })),
          { value: "custom", label: "Tuyến tùy chọn trên bản đồ", disabled: true },
        ]}
        onChange={ride.preset}
        isDisabled={disabled}
        width="100%"
      />
      <HStack gap={3} wrap="wrap">
        <Button
          label="Chọn điểm đón"
          icon={<SiteIcon name="map-pin" />}
          variant={form.target === 0 ? "primary" : "secondary"}
          onClick={() => updateForm({ target: 0 })}
          isDisabled={disabled}
        />
        <Button
          label="Chọn điểm đến"
          icon={<SiteIcon name="flag-checkered" />}
          variant={form.target === 1 ? "primary" : "secondary"}
          onClick={() => updateForm({ target: 1 })}
          isDisabled={disabled}
        />
        <Button
          label="Đảo chiều"
          icon={<SiteIcon name="arrows-down-up" />}
          variant="secondary"
          onClick={ride.swap}
          isDisabled={disabled}
        />
      </HStack>
      <Text as="p" type="supporting" aria-live="polite">
        {`Chạm bản đồ để chọn ${form.target === 0 ? "điểm đón" : "điểm đến"}.`}
      </Text>
      <MapToolbar
        location={location}
        busy={busy}
        appearance={appearance}
        onAppearance={setAppearance}
      />
      <RouteMap
        points={form.points}
        geometry={geometry}
        onSelect={ride.chooseLocation}
        isDisabled={disabled}
        appearance={appearance}
      />
      {form.points.map((point, index) => (
        <VStack key={point.type} gap={1} className="selected-location">
          <HStack gap={2} vAlign="center">
            <SiteIcon name={index === 0 ? "map-pin" : "flag-checkered"} size="sm" color="accent" />
            <Text type="label">{index === 0 ? "Điểm đón" : "Điểm đến"}</Text>
          </HStack>
          <Text as="p">{point.address}</Text>
        </VStack>
      ))}
      <AddressSearch ride={ride} disabled={disabled} />
    </VStack>
  );
}

function MapToolbar({ location, busy, appearance, onAppearance }) {
  return (
    <VStack gap={3}>
      <HStack gap={3} wrap="wrap" className="map-toolbar" vAlign="end">
        <Selector
          label="Nền bản đồ"
          startIcon={<SiteIcon name="map-pin" />}
          value={appearance}
          onChange={onAppearance}
          options={[
            { value: "light", label: "Gọn nhẹ" },
            { value: "dark", label: "Ban đêm" },
            { value: "standard", label: "Đường phố" },
          ]}
        />
        <Button
          label="Lấy vị trí của tôi"
          icon={<SiteIcon name="navigation-arrow" />}
          variant="secondary"
          onClick={location.locate}
          isLoading={location.busy}
          isDisabled={busy}
        />
      </HStack>
      {location.error && (
        <Banner status="error" title="Không lấy được vị trí" description={location.error} />
      )}
      {location.accuracy !== null && (
        <Text as="p" type="supporting" aria-live="polite">
          {`Đã chọn vị trí của bạn · sai số khoảng ${location.accuracy} m.`}
        </Text>
      )}
    </VStack>
  );
}

function AddressSearch({ ride, disabled }) {
  const { form, updateForm } = ride;
  return (
    <VStack gap={3}>
      <TextInput
        label={form.target === 0 ? "Tìm địa chỉ đón" : "Tìm địa chỉ đến"}
        startIcon="search"
        value={form.query}
        onChange={(query) => updateForm({ query })}
        isDisabled={disabled}
        placeholder="Ví dụ: Văn Miếu, Hà Nội"
      />
      <Button
        label="Tìm địa chỉ"
        icon={<SiteIcon name="map-pin" />}
        variant="secondary"
        onClick={ride.search}
        isDisabled={disabled}
      />
      {ride.places.length > 0 && (
        <Selector
          label="Chọn địa chỉ"
          startIcon={<SiteIcon name="map-pin" />}
          placeholder={`${ride.places.length} địa chỉ tìm thấy`}
          options={ride.places.map((place, index) => ({
            value: String(index),
            label: place.label,
            description: `${place.latitude.toFixed(5)}, ${place.longitude.toFixed(5)}`,
          }))}
          onChange={(value) => {
            const place = ride.places[Number(value)];
            if (place) {
              ride.chooseLocation(place);
              updateForm({ query: place.label });
            }
          }}
          isDisabled={disabled}
          width="100%"
          hasSearch
          presentation="popover"
          searchPlaceholder="Lọc địa chỉ"
          emptySearchText="Không có địa chỉ phù hợp"
        />
      )}
    </VStack>
  );
}

function PassengerControls({ ride, busy }) {
  const { form, updateForm } = ride;
  return (
    <VStack gap={4}>
      <Grid columns={2} gap={3} className="credentials-grid">
        <TextInput
          label="Tên khách"
          startIcon={<SiteIcon name="user" />}
          value={form.name}
          onChange={(name) => updateForm({ name })}
          isDisabled={busy}
        />
        <TextInput
          label="Số điện thoại khách"
          startIcon={<SiteIcon name="phone" />}
          value={form.phone}
          onChange={(phone) => updateForm({ phone })}
          isDisabled={busy}
        />
      </Grid>
      <Grid columns={2} gap={3} className="credentials-grid">
        <Selector
          label="Loại xe"
          startIcon={<SiteIcon name={form.serviceId === 2179 ? "car" : "motorcycle"} />}
          value={String(form.serviceId)}
          options={[
            { value: "2179", label: "Ô tô" },
            { value: "2180", label: "Xe máy" },
          ]}
          onChange={(id) => updateForm({ serviceId: Number(id) })}
          isDisabled={busy}
        />
        <Selector
          label="Thanh toán"
          startIcon={<SiteIcon name="wallet" />}
          value={form.payment}
          options={[
            { value: "cash", label: "Tiền mặt" },
            { value: "partner_pay", label: "Đối tác thanh toán" },
          ]}
          onChange={(payment) => updateForm({ payment })}
          isDisabled={busy}
        />
      </Grid>
      <Button
        label="So sánh giá"
        icon={<SiteIcon name="currency-circle-dollar" />}
        onClick={ride.compare}
        isDisabled={busy}
        width="100%"
      />
    </VStack>
  );
}

function FareComparison({ ride, busy }) {
  const estimates = quoteEstimates(ride.quote?.result.data);
  const selected = estimates.find((item) => item.service.id === ride.form.serviceId);
  const info = selected?.estimate_info;
  const vehicleName = ride.form.serviceId === 2179 ? "ô tô" : "xe máy";
  if (!ride.quote)
    return (
      <Text as="p" color="secondary">
        Chọn hai điểm trên bản đồ rồi bấm So sánh giá.
      </Text>
    );
  const rows = estimates.map((item) => ({
    id: item.service.id,
    service: item.service.travel_mode === "Car" ? "Ô tô" : "Xe máy",
    fare: item.fee_unavailable ? item.err_msg : money(item.estimate_info.estimate.total_pay),
  }));
  return (
    <VStack gap={4} aria-live="polite">
      <Table
        data={rows}
        idKey="id"
        columns={[
          { key: "service", header: "Loại xe" },
          { key: "fare", header: "Giá dự kiến" },
        ]}
        density="compact"
      />
      {info && (
        <>
          <Heading level={3}>{`Giá ${vehicleName}: ${money(info.estimate.total_pay)}`}</Heading>
          <Text as="p">{routeSummary(info.trip)}</Text>
          <Table
            data={info.pricing.items.map(fareLineRow)}
            idKey="label"
            columns={[
              { key: "label", header: "Khoản cước" },
              { key: "calculation", header: "Cách tính" },
              { key: "amount", header: "Thành tiền" },
            ]}
            density="compact"
          />
          <Button
            label="Đặt xe"
            icon={<SiteIcon name="car" />}
            onClick={ride.book}
            isDisabled={busy}
            width="100%"
          />
        </>
      )}
    </VStack>
  );
}

function driverSummary(order) {
  if (!order.driver?.id) return "Chưa gán tài xế. Đơn đang tìm tài xế.";
  const vehicle = order.vehicle ?? {};
  const name = order.driver.id === "local-driver" ? "Tài xế mô phỏng" : order.driver.name;
  const plate = vehicle.license_plate?.replace(" (TEST)", "") ?? "";
  return `${name} · ${vehicle.model} · ${plate}`;
}

function OrderButtons({ ride, model }) {
  const enabled = simulationActions(ride.order.status);
  const canSimulate = !model.state.busy;
  const actions = [
    {
      label: "Gán tài xế",
      icon: "user-circle",
      run: () => ride.simulate("ASSIGNED"),
      enabled: canSimulate && enabled.assign,
    },
    {
      label: "Bắt đầu chuyến",
      icon: "play",
      run: () => ride.simulate("IN PROCESS"),
      enabled: canSimulate && enabled.start,
    },
    {
      label: "Hoàn thành chuyến",
      icon: "check-circle",
      run: () => ride.simulate("COMPLETED"),
      enabled: canSimulate && enabled.complete,
    },
    {
      label: "Hủy chuyến",
      icon: "x-circle",
      run: ride.cancel,
      enabled: !model.state.busy && enabled.cancel,
    },
    {
      label: "Tải lại chuyến",
      icon: "arrows-clockwise",
      run: ride.refresh,
      enabled: !model.state.busy,
    },
  ];
  return (
    <HStack gap={3} wrap="wrap">
      {actions.map((action) => (
        <Button
          key={action.label}
          label={action.label}
          icon={<SiteIcon name={action.icon} />}
          variant="secondary"
          onClick={action.run}
          isDisabled={!action.enabled}
        />
      ))}
    </HStack>
  );
}

function OrderControls({ ride, model }) {
  const { order } = ride;
  if (!order) return null;
  const fare = order.fare?.estimate;
  return (
    <VStack gap={4} className="quick-order" aria-live="polite">
      <Divider />
      <Heading level={3}>{orderStatusNames[order.status] ?? "Chuyến đi"}</Heading>
      <Text as="p" type="supporting" className="order-reference">{`Mã chuyến: ${order.id}`}</Text>
      {fare && <Text as="p">{`Giá đã chốt: ${money(fare.total_pay)}`}</Text>}
      <Text as="p">{driverSummary(order)}</Text>
      <Text as="p" type="supporting">
        Sandbox tự chạy chuyến và cập nhật trạng thái. Mỗi bước khoảng 5 giây.
      </Text>
      <OrderButtons ride={ride} model={model} />
    </VStack>
  );
}

export default function QuickRide({ model }) {
  const ride = useQuickRide(model);
  const { state } = model;
  const geometry = quoteGeometry(ride.quote);
  return (
    <VStack gap={6}>
      <HStack gap={4} hAlign="between" wrap="wrap">
        <VStack gap={2}>
          <HStack gap={2} vAlign="center">
            <SiteIcon name="car" color="accent" />
            <Heading level={2}>Chuyến đi mới</Heading>
          </HStack>
          <Text as="p" color="secondary">
            Chọn điểm đón và điểm đến để xem giá.
          </Text>
        </VStack>
      </HStack>
      <Grid columns={2} gap={6} align="start" className="playground-grid">
        <VStack gap={5} className="quick-column">
          <PointControls ride={ride} busy={state.busy} geometry={geometry} />
        </VStack>
        <VStack gap={5} className="quick-column">
          <PassengerControls ride={ride} busy={state.busy} />
          {ride.error && (
            <Banner status="error" title="Chưa thực hiện được" description={ride.error} />
          )}
          {state.busy && <Text as="output">Đang gọi API…</Text>}
          <FareComparison ride={ride} busy={state.busy} />
          <Text as="p" type="supporting" color="secondary">
            Giá tham khảo tại Hà Nội, tháng 3/2025. Chuyến đi và tài xế được mô phỏng.
          </Text>
          <Link
            href="https://www.xanhsm.com/news/du-lich-sinh-thai-ban-rom-soc-son-ha-noi"
            type="supporting"
            target="_blank"
            rel="noreferrer"
          >
            <HStack gap={2} vAlign="center">
              Nguồn bảng giá tham khảo
              <SiteIcon name="arrow-right" size="sm" />
            </HStack>
          </Link>
          <OrderControls ride={ride} model={model} />
        </VStack>
      </Grid>
    </VStack>
  );
}
