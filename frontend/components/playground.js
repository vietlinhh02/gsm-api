"use client";

import { useState } from "react";
import { useClipboard } from "@astryxdesign/core/hooks";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Card } from "@astryxdesign/core/Card";
import { CodeBlock } from "@astryxdesign/core/CodeBlock";
import { Grid } from "@astryxdesign/core/Grid";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Heading, Text } from "@astryxdesign/core/Text";
import { Link } from "@astryxdesign/core/Link";
import { Selector } from "@astryxdesign/core/Selector";
import { TextArea } from "@astryxdesign/core/TextArea";
import { TextInput } from "@astryxdesign/core/TextInput";
import { TabList, Tab } from "@astryxdesign/core/TabList";
import { endpoints } from "#lib/catalog.mjs";
import { usePlayground } from "@/components/use-playground";
import QuickRide from "@/components/quick-ride";
import { apiErrorMessage } from "#lib/http.mjs";
import SiteIcon from "@/components/site-icon";

function CopyToken({ token, busy }) {
  const { copy, isCopied } = useClipboard({ announce: "Đã sao chép token" });
  const [failed, setFailed] = useState(false);
  return (
    <VStack gap={2}>
      <Button
        label={isCopied ? "Đã sao chép token" : "Sao chép token"}
        variant="secondary"
        isDisabled={!token || busy}
        onClick={async () => setFailed(!(await copy(token)))}
      />
      {failed && (
        <Text as="p" role="alert">
          Không sao chép được. Chọn nội dung ô Bearer token rồi sao chép thủ công.
        </Text>
      )}
    </VStack>
  );
}

function Authentication({ model }) {
  const { state, patch, authenticate } = model;
  return (
    <VStack gap={4}>
      <HStack gap={2} vAlign="center">
        <SiteIcon name="key" color="accent" />
        <Heading level={3}>Xác thực</Heading>
      </HStack>
      <Text as="p" color="secondary">
        Để trống Client ID và Client secret để dùng cấu hình máy chủ của playground. Điền cả hai nếu
        bạn muốn dùng thông tin xác thực riêng.
      </Text>
      <Grid columns={2} gap={3} className="credentials-grid">
        <TextInput
          label="Client ID"
          startIcon={<SiteIcon name="user-circle" />}
          value={state.clientId}
          onChange={(value) => patch({ clientId: value })}
          isDisabled={state.busy}
        />
        <TextInput
          label="Client secret"
          startIcon={<SiteIcon name="key" />}
          type="password"
          value={state.clientSecret}
          autoComplete="off"
          onChange={(value) => patch({ clientSecret: value })}
          isDisabled={state.busy}
        />
      </Grid>
      <Button
        label="Lấy access token"
        icon={<SiteIcon name="key" />}
        variant="secondary"
        isLoading={state.busy}
        onClick={authenticate}
      />
      <TextInput
        label="Bearer token"
        startIcon={<SiteIcon name="shield-check" />}
        type="password"
        value={state.token}
        placeholder="Lấy hoặc dán access token"
        autoComplete="off"
        onChange={(value) => patch({ token: value })}
        isDisabled={state.busy}
      />
      <CopyToken key={state.token} token={state.token} busy={state.busy} />
    </VStack>
  );
}

function RequestPanel({ model }) {
  const { state, endpoint, patch, selectEndpoint, send } = model;
  const options = endpoints
    .filter((item) => !item.sandboxOnly || state.environment === "sandbox")
    .map((item) => ({ value: item.id, label: item.method + " · " + item.title }));
  return (
    <Card padding={6} className="request-panel">
      <VStack gap={5}>
        <HStack hAlign="between" vAlign="center">
          <HStack gap={2} vAlign="center">
            <SiteIcon name="paper-plane" color="accent" />
            <Heading level={2}>Request</Heading>
          </HStack>
          <Link href={"/docs/" + endpoint.id} type="supporting">
            <HStack gap={2} vAlign="center">
              <SiteIcon name="book-open" size="sm" />
              Tài liệu
            </HStack>
          </Link>
        </HStack>
        <Selector
          label="Endpoint"
          startIcon={<SiteIcon name="code" />}
          options={options}
          value={endpoint.id}
          onChange={selectEndpoint}
          width="100%"
          isDisabled={state.busy}
          hasSearch
          searchPlaceholder="Tìm endpoint"
          emptySearchText="Không tìm thấy endpoint phù hợp."
          presentation="adaptive"
        />
        <TextInput
          label="Request URL"
          startIcon={<SiteIcon name="plugs" />}
          value={state.path}
          onChange={(value) => patch({ path: value })}
          isDisabled={state.busy}
          containerClassName="path-input"
        />
        {endpoint.method === "POST" && (
          <TextArea
            label="Request body (JSON)"
            rows={14}
            value={state.body}
            onChange={(value) => patch({ body: value })}
            hasSpellCheck={false}
            isDisabled={state.busy}
            containerClassName="json-editor"
          />
        )}
        <Button
          label="Gửi request"
          icon={<SiteIcon name="paper-plane" />}
          variant="primary"
          width="100%"
          onClick={send}
          isLoading={state.busy}
        />
      </VStack>
    </Card>
  );
}

function ResultData({ result }) {
  const isError = result.status >= 400;
  return (
    <VStack gap={4}>
      <Text type="label" className={isError ? "response-error" : "response-success"}>
        {"HTTP " + result.status}
      </Text>
      <Text as="p" type="supporting" className="path-text">
        {result.requestMethod + " " + result.requestPath}
      </Text>
      {isError && (
        <Banner status="error" title="API trả về lỗi" description={apiErrorMessage(result)} />
      )}
      <CodeBlock
        code={result.text}
        language={result.data ? "json" : "plaintext"}
        width="100%"
        title="Response body"
        isWrapped
        maxHeight="calc(var(--spacing-10) * 15)"
      />
    </VStack>
  );
}

function ResponseContent({ state }) {
  if (state.error)
    return <Banner status="error" title="Không gửi được request" description={state.error} />;
  if (state.result) return <ResultData result={state.result} />;
  return (
    <VStack gap={3} className="response-empty" padding={6}>
      <Heading level={3}>{state.busy ? "Đang gửi…" : "Chưa có kết quả"}</Heading>
      <Text as="p" color="secondary">
        Chọn endpoint rồi gửi request. Response sẽ hiện ở đây.
      </Text>
    </VStack>
  );
}

function BookingNextStep({ state, selectEndpoint }) {
  const canBook =
    state.result?.status === 200 && state.booking && ["trip", "hour"].includes(state.endpointId);
  if (!canBook) return null;
  return (
    <Banner
      status="success"
      title="Báo giá đã sẵn sàng"
      description="Thông tin báo giá đã được điền vào request đặt chuyến."
      endContent={
        <Button
          label="Dùng báo giá để đặt"
          icon={<SiteIcon name="car" />}
          variant="secondary"
          onClick={() => selectEndpoint("create")}
        />
      }
    />
  );
}

function ResponsePanel({ model }) {
  const { state, selectEndpoint } = model;
  return (
    <Card padding={6} className="response-panel">
      <VStack gap={5} aria-live="polite" aria-atomic="false">
        <HStack hAlign="between" vAlign="center">
          <HStack gap={2} vAlign="center">
            <SiteIcon name="code" color="accent" />
            <Heading level={2}>Response</Heading>
          </HStack>
          {state.result && <Text type="supporting">{state.result.duration} ms</Text>}
        </HStack>
        <ResponseContent state={state} />
        <BookingNextStep state={state} selectEndpoint={selectEndpoint} />
        {state.orderId && (
          <Text type="supporting" className="order-reference">
            {"Chuyến gần nhất: " + state.orderId}
          </Text>
        )}
      </VStack>
    </Card>
  );
}

export default function Playground({ endpointId }) {
  const model = usePlayground(endpointId);
  const [tab, setTab] = useState(endpointId ? "api" : "ride");
  return (
    <VStack gap={8} padding={8} className="playground-page">
      <HStack hAlign="between" gap={6} vAlign="end" wrap="wrap">
        <VStack gap={3}>
          <Text type="supporting" color="secondary">
            QUẢN LÝ CHUYẾN XE
          </Text>
          <Heading level={1} type="display-2">
            API playground
          </Heading>
          <Text as="p" type="large" color="secondary">
            Xem giá, đặt xe và theo dõi chuyến đi.
          </Text>
        </VStack>
        <Selector
          label="Môi trường"
          startIcon={<SiteIcon name={model.state.environment === "sandbox" ? "flask" : "plugs"} />}
          value={model.state.environment}
          options={[
            { value: "production", label: "Production" },
            { value: "sandbox", label: "Sandbox" },
          ]}
          onChange={model.selectEnvironment}
          isDisabled={model.state.busy}
        />
      </HStack>
      <TabList
        value={tab}
        onChange={setTab}
        role="tablist"
        aria-label="Công cụ chuyến xe"
        hasDivider
        layout="fill"
      >
        <Tab
          value="ride"
          label="Đặt chuyến"
          icon={<SiteIcon name="car" />}
          panelId="playground-ride"
        />
        <Tab
          value="api"
          label="API & JSON"
          icon={<SiteIcon name="code" />}
          panelId="playground-api"
        />
        <Tab
          value="auth"
          label="Xác thực"
          icon={<SiteIcon name="key" />}
          panelId="playground-auth"
        />
      </TabList>
      <section id="playground-ride" role="tabpanel" aria-label="Đặt chuyến" hidden={tab !== "ride"}>
        <QuickRide key={model.state.environment} model={model} />
      </section>
      <section id="playground-api" role="tabpanel" aria-label="API & JSON" hidden={tab !== "api"}>
        <Grid columns={2} gap={6} align="start" className="playground-grid">
          <RequestPanel model={model} />
          <ResponsePanel model={model} />
        </Grid>
      </section>
      <section id="playground-auth" role="tabpanel" aria-label="Xác thực" hidden={tab !== "auth"}>
        <Card padding={6} className="auth-panel">
          <Authentication model={model} />
        </Card>
      </section>
    </VStack>
  );
}
