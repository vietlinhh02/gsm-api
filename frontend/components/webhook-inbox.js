"use client";

import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { CodeBlock } from "@astryxdesign/core/CodeBlock";
import { Divider } from "@astryxdesign/core/Divider";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Table } from "@astryxdesign/core/Table";
import { Heading, Text } from "@astryxdesign/core/Text";
import { TextInput } from "@astryxdesign/core/TextInput";
import { receiptContent } from "#lib/webhook-inbox.mjs";
import { useWebhookInbox } from "@/components/use-webhook-inbox";
import SiteIcon from "@/components/site-icon";

function InboxLink({ inbox, copy, busy }) {
  return (
    <VStack gap={3}>
      <TextInput label="Link nhận webhook" value={inbox.receive_url} isReadOnly width="100%" />
      <HStack gap={3} wrap="wrap">
        <Button
          label="Sao chép link"
          icon={<SiteIcon name="plugs" />}
          onClick={copy}
          variant="secondary"
          isDisabled={busy}
        />
        <Text as="p" type="supporting">
          Chuyến mới trong môi trường này sẽ gửi webhook vào hộp thư đang chọn.
        </Text>
      </HStack>
    </VStack>
  );
}

function InboxReceipts({ inbox }) {
  if (inbox.receipts.length === 0) {
    return (
      <Text as="p" color="secondary">
        Chưa nhận request. Gửi event thử hoặc tạo một chuyến mới.
      </Text>
    );
  }
  const rows = inbox.receipts.map((receipt) => ({
    ...receipt,
    time: new Date(receipt.received_at).toLocaleString("vi-VN"),
    size: `${receipt.size.toLocaleString("vi-VN")} byte`,
  }));
  return (
    <Table
      data={rows}
      idKey="id"
      density="compact"
      textOverflow="truncate"
      columns={[
        { key: "time", header: "Thời gian nhận" },
        { key: "preview", header: "Nội dung" },
        { key: "size", header: "Dung lượng" },
        {
          key: "id",
          header: "Chi tiết",
          renderCell: (row) => (
            <Button
              label={`Xem #${row.id}`}
              variant="secondary"
              onClick={() => inbox.select(row.id)}
              isDisabled={inbox.busy}
            />
          ),
        },
      ]}
    />
  );
}

function ReceiptDetails({ receipt }) {
  if (!receipt) return null;
  const content = receiptContent(receipt.body);
  return (
    <VStack gap={4}>
      <Divider />
      <Heading level={3}>{`Request #${receipt.id}`}</Heading>
      <CodeBlock
        title="Headers"
        code={JSON.stringify(receipt.headers, null, 2)}
        language="json"
        width="100%"
        isWrapped
      />
      <CodeBlock
        title="Request body"
        code={content.code}
        language={content.language}
        width="100%"
        isWrapped
        maxHeight="calc(var(--spacing-10) * 15)"
      />
    </VStack>
  );
}

export default function WebhookInbox({ model, active }) {
  const inbox = useWebhookInbox(model, active);
  const disabled = inbox.busy || model.state.busy;
  return (
    <VStack gap={5}>
      <Heading level={2}>Hộp thư webhook</Heading>
      <Text as="p" color="secondary">
        Tạo link nhận webhook để thử tích hợp. Xem các request gửi từ chuyến đi hoặc hệ thống của
        bạn.
      </Text>
      <InboxActions inbox={inbox} disabled={disabled} />
      {inbox.error && (
        <Banner status="error" title="Chưa thực hiện được" description={inbox.error} />
      )}
      {inbox.notice && (
        <Text as="p" aria-live="polite">
          {inbox.notice}
        </Text>
      )}
      {inbox.inbox && <InboxLink inbox={inbox.inbox} copy={inbox.copy} busy={disabled} />}
      {inbox.inbox && (
        <VStack gap={4}>
          <Text as="p" type="supporting">
            100 request gần nhất · Tự cập nhật mỗi 3 giây.
          </Text>
          <InboxReceipts inbox={inbox} />
          <ReceiptDetails receipt={inbox.selected} />
        </VStack>
      )}
    </VStack>
  );
}

function InboxActions({ inbox, disabled }) {
  return (
    <HStack gap={3} wrap="wrap">
      <Button
        label={inbox.inbox ? "Tạo link mới" : "Tạo link webhook"}
        icon={<SiteIcon name="plugs" />}
        onClick={inbox.create}
        isDisabled={disabled}
      />
      {inbox.inbox && (
        <Button
          label="Gửi event thử"
          icon={<SiteIcon name="paper-plane" />}
          onClick={inbox.test}
          variant="secondary"
          isDisabled={disabled}
        />
      )}
      {inbox.inbox && (
        <Button
          label="Tải lại hộp thư"
          icon={<SiteIcon name="arrows-clockwise" />}
          onClick={inbox.refresh}
          variant="secondary"
          isDisabled={disabled}
        />
      )}
    </HStack>
  );
}
