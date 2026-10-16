import { Button } from "@astryxdesign/core/Button";
import DocumentationCode from "@/components/documentation-code";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Grid } from "@astryxdesign/core/Grid";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Heading, Text } from "@astryxdesign/core/Text";
import { Link } from "@astryxdesign/core/Link";
import { Table } from "@astryxdesign/core/Table";
import { endpoints, guides, pointFields } from "#lib/catalog.mjs";
import { endpointDocumentation, guideDocumentation } from "#lib/documentation.mjs";
import {
  DescriptionTable,
  EndpointUsage,
  RequestExamples,
  ErrorReference,
  RelatedDocuments,
  GuideSections,
  DocumentationCopy,
} from "@/components/documentation-sections";

const fieldColumns = [
  { key: "name", header: "Tham số" },
  { key: "type", header: "Kiểu dữ liệu" },
  { key: "required", header: "Yêu cầu" },
  { key: "description", header: "Mô tả" },
];

export function DocFrame({ title, children, sections = [] }) {
  return (
    <Grid columns={sections.length ? 2 : 1} gap={6} padding={6} className="docs-page">
      <VStack as="article" gap={6} className="article">
        <HStack gap={2} vAlign="center">
          <Link href="/" type="supporting" color="secondary">
            Tài liệu
          </Link>
          <Text type="supporting" color="secondary">
            /
          </Text>
          <Text type="supporting">{title}</Text>
        </HStack>
        {children}
        <Divider />
        <Text type="supporting" color="secondary">
          API đặt xe GreenSM · Dữ liệu bản đồ © OpenStreetMap
        </Text>
      </VStack>
      {sections.length > 0 && (
        <VStack as="aside" aria-label="Trên trang này" gap={2} className="article-outline">
          <Text type="label">Trên trang này</Text>
          {sections.map((section) => (
            <Link key={section.id} href={"#" + section.id} color="secondary" type="supporting">
              {section.title}
            </Link>
          ))}
          <Divider />
          <Link href="/playground" type="supporting">
            Mở playground
          </Link>
        </VStack>
      )}
    </Grid>
  );
}

export function EndpointDocument({ endpoint }) {
  const documentation = endpointDocumentation[endpoint.id];
  const sections = [
    { id: "usage", title: "Cách sử dụng" },
    { id: "params", title: "Tham số" },
    { id: "request", title: "Request" },
    { id: "response", title: "Response" },
    { id: "errors", title: "Lỗi và cách xử lý" },
    { id: "next", title: "Bước tiếp theo" },
  ];
  return (
    <DocFrame title={endpoint.title} sections={sections}>
      <VStack gap={3}>
        <Text type="supporting" color="secondary">
          {endpoint.group} / API v1
        </Text>
        <Heading level={1} type="display-2">
          {endpoint.title}
        </Heading>
        <Text as="p" color="secondary" className="doc-copy">
          {endpoint.description}
        </Text>
        <HStack gap={3} padding={3} vAlign="center" className="endpoint-path">
          <Text type="label" className={"http-method " + endpoint.method.toLowerCase()}>
            {endpoint.method}
          </Text>
          <Text type="code" className="path-text">
            {endpoint.path}
          </Text>
        </HStack>
        <Button
          label="Mở endpoint trong playground"
          href={"/playground?endpoint=" + endpoint.id}
          variant="primary"
        />
      </VStack>
      <EndpointUsage endpoint={endpoint} documentation={documentation} />
      <EndpointParameters endpoint={endpoint} />
      <EndpointExamples endpoint={endpoint} documentation={documentation} />
      <ErrorReference errors={documentation.errors} />
      <RelatedDocuments ids={documentation.next} />
    </DocFrame>
  );
}

function EndpointParameters({ endpoint }) {
  return (
    <VStack gap={3} as="section" id="params">
      <Heading level={2}>Tham số</Heading>
      {endpoint.fields.length ? (
        <Table
          data={endpoint.fields}
          columns={fieldColumns}
          idKey="name"
          density="compact"
          verticalAlign="top"
        />
      ) : (
        <Text as="p" color="secondary">
          Không cần request body.
        </Text>
      )}
      {endpoint.hasPoints && (
        <VStack gap={3}>
          <Heading level={3}>Thông tin điểm đón và điểm đến</Heading>
          <Table
            data={pointFields}
            columns={fieldColumns}
            idKey="name"
            verticalAlign="top"
            density="compact"
          />
        </VStack>
      )}
    </VStack>
  );
}

function EndpointExamples({ endpoint, documentation }) {
  return (
    <VStack gap={6}>
      <VStack gap={3} as="section" id="request">
        <Heading level={2}>Request</Heading>
        <DocumentationCode
          code={
            endpoint.sample
              ? JSON.stringify(endpoint.sample, null, 2)
              : endpoint.method + " " + endpoint.path
          }
          title={endpoint.sample ? "Request body" : "Request URL"}
          language={endpoint.sample ? "json" : "plaintext"}
          width="100%"
        />
        <DocumentationCopy paragraphs={endpoint.notes} />
        <RequestExamples endpoint={endpoint} />
      </VStack>
      <VStack gap={3} as="section" id="response">
        <Heading level={2}>Response</Heading>
        <Text color="secondary">
          Ví dụ dưới đây gồm các trường chính. Giá, mã chuyến và thời gian tùy theo yêu cầu.
        </Text>
        <DocumentationCode
          code={JSON.stringify(endpoint.response, null, 2)}
          title="200 OK"
          language="json"
          width="100%"
        />
        <DescriptionTable rows={documentation.responseFields} />
      </VStack>
    </VStack>
  );
}

export function GuideDocument({ guide, guideId }) {
  const documentation = guideDocumentation[guideId];
  return (
    <DocFrame
      title={guide.title}
      sections={[
        ...documentation.sections.map(({ id, title }) => ({ id, title })),
        { id: "example", title: "Ví dụ tích hợp" },
      ]}
    >
      <VStack gap={2}>
        <Heading level={1} type="display-2">
          {guide.title}
        </Heading>
        <Text as="p" color="secondary">
          {guide.description}
        </Text>
      </VStack>
      <DocumentationCopy paragraphs={guide.paragraphs} />
      <GuideSections sections={documentation.sections} />
      <VStack gap={3} as="section" id="example">
        <Heading level={2}>Ví dụ tích hợp</Heading>
        <DocumentationCode
          code={guide.code}
          title="Ví dụ tích hợp"
          language={guide.language}
          width="100%"
        />
      </VStack>
      <Button label="Mở playground" href="/playground" variant="primary" />
    </DocFrame>
  );
}

export function Overview() {
  return (
    <DocFrame
      title="API đặt xe"
      sections={[
        { id: "start", title: "Bắt đầu tích hợp" },
        { id: "environments", title: "Môi trường test" },
        { id: "endpoints", title: "Danh sách endpoint" },
      ]}
    >
      <VStack gap={3}>
        <Text type="supporting" weight="semibold" color="secondary">
          TÀI LIỆU TÍCH HỢP
        </Text>
        <Heading level={1} type="display-2">
          API đặt xe
        </Heading>
        <Text as="p" type="large" color="secondary" className="hero-description">
          Tìm loại xe, lấy báo giá, đặt chuyến và theo dõi trạng thái qua API.
        </Text>
        <HStack gap={3} wrap="wrap">
          {["REST / JSON", "OAuth 2.0", "Việt Nam · Phiên bản 1"].map((label) => (
            <Text type="supporting" key={label}>
              {label}
            </Text>
          ))}
        </HStack>
        <HStack gap={3} wrap="wrap">
          <Button href="/docs/auth" label="Bắt đầu tích hợp" variant="primary" />
          <Button href="/playground" label="Mở playground" variant="secondary" />
        </HStack>
      </VStack>
      <Card variant="muted">
        <VStack gap={2}>
          <Heading level={3}>Về dữ liệu</Heading>
          <Text as="p" color="secondary">
            Chuyến đi và tài xế được mô phỏng. Địa chỉ và tuyến đường lấy từ OpenStreetMap.
          </Text>
        </VStack>
      </Card>
      <QuickStart />
      <EnvironmentOverview />
      <Card variant="muted">
        <VStack gap={3}>
          <Heading level={3}>Xác thực ứng dụng</Heading>
          <Text as="p" color="secondary">
            Gửi Client ID và Client secret để lấy access token trước khi gọi API.
          </Text>
          <DocumentationCode
            code={guides.auth.code}
            title="Lấy access token"
            language="bash"
            width="100%"
            container="section"
          />
        </VStack>
      </Card>
      <VStack as="section" id="endpoints" gap={3}>
        <Heading level={2}>Danh sách endpoint</Heading>
        <Text as="p" color="secondary">
          Tất cả endpoint test dùng chung base URL /v1/sandbox.
        </Text>
        <Table
          data={endpoints.map(({ id, method, path, title }) => ({ id, method, path, title }))}
          columns={[
            { key: "method", header: "HTTP method" },
            { key: "path", header: "Endpoint" },
            { key: "title", header: "Mô tả" },
          ]}
          idKey="id"
          verticalAlign="top"
          density="compact"
        />
      </VStack>
    </DocFrame>
  );
}

function QuickStart() {
  return (
    <VStack gap={3} as="section" id="start">
      <Heading level={2}>Bắt đầu tích hợp</Heading>
      <Text as="p" color="secondary">
        Lấy access token, lấy báo giá rồi gửi request đặt chuyến.
      </Text>
      <VStack gap={3}>
        {[
          ["01", "Lấy access token", "Xác thực bằng Client ID và Client secret.", "/docs/auth"],
          [
            "02",
            "Chọn dịch vụ và nhận báo giá",
            "Giữ lại fee_id và thông tin báo giá để đặt chuyến.",
            "/docs/trip",
          ],
          [
            "03",
            "Tạo và theo dõi chuyến",
            "Tra cứu chuyến đi hoặc nhận thông báo qua webhook.",
            "/docs/create",
          ],
        ].map(([number, title, description, href]) => (
          <HStack key={number} gap={3} vAlign="start" className="quick-step">
            <Text type="code" color="secondary" className="step-number">
              {number}
            </Text>
            <VStack gap={1}>
              <Link href={href} color="primary" weight="semibold">
                {title}
              </Link>
              <Text color="secondary">{description}</Text>
            </VStack>
          </HStack>
        ))}
      </VStack>
    </VStack>
  );
}

function EnvironmentOverview() {
  return (
    <VStack gap={3} as="section" id="environments">
      <Heading level={2}>Môi trường test</Heading>
      <Text as="p" color="secondary">
        Ứng dụng chỉ dùng Sandbox để mô phỏng chuyến xe và tài xế. Giá là bảng tham khảo Hà Nội
        tháng 3/2025.
      </Text>
      <DescriptionTable
        rows={[
          ["Sandbox", "Ride endpoint: /v1/sandbox/ride/ · Token: /v1/sandbox/token"],
          [
            "Sandbox scope",
            "sandbox.ride.trips; dùng update-status để gán tài xế và đổi trạng thái.",
          ],
          [
            "Timestamp",
            "Request dùng Unix giây dạng số; nhiều trường thời gian trả về dạng chuỗi.",
          ],
          ["Đơn vị", "Tiền theo VND, quãng đường theo km, thời gian tuyến đường theo giây."],
        ]}
      />
    </VStack>
  );
}
