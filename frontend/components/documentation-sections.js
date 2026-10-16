import { Button } from "@astryxdesign/core/Button";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Heading, Text } from "@astryxdesign/core/Text";
import { Table } from "@astryxdesign/core/Table";
import DocumentationCode from "@/components/documentation-code";
import { endpoints, guides } from "#lib/catalog.mjs";
import { commonErrors } from "#lib/documentation.mjs";

const descriptionColumns = [
  { key: "name", header: "Trường" },
  { key: "description", header: "Ý nghĩa" },
];

export function DescriptionTable({ rows }) {
  return (
    <Table
      data={rows.map(([name, description]) => ({ name, description }))}
      columns={descriptionColumns}
      idKey="name"
      verticalAlign="top"
      density="compact"
    />
  );
}

export function EndpointUsage({ endpoint, documentation }) {
  const sandboxPath = endpoint.sandboxOnly
    ? endpoint.path
    : endpoint.path.replace("/v1/", "/v1/sandbox/");
  return (
    <VStack gap={3} as="section" id="usage">
      <Heading level={2}>Cách sử dụng</Heading>
      <DocumentationCopy paragraphs={documentation.usage} />
      <Text as="p" type="supporting" color="secondary">
        {endpoint.sandboxOnly
          ? "Dùng access token có scope sandbox.ride.trips."
          : "Production dùng scope ride.trips; Sandbox dùng scope sandbox.ride.trips."}
      </Text>
      <DocumentationCode code={sandboxPath} title="Sandbox URL" language="plaintext" width="100%" />
    </VStack>
  );
}

export function RequestExamples({ endpoint }) {
  const lines = [
    'API_BASE_URL="http://localhost:8081"',
    'ACCESS_TOKEN="<access_token>"',
    "",
    `curl -X ${endpoint.method} "$API_BASE_URL${endpoint.path}" \\`,
    '  -H "Authorization: Bearer $ACCESS_TOKEN" \\',
    "  -H 'Accept-Language: vi'",
  ];
  if (endpoint.sample) {
    lines[lines.length - 1] += " \\";
    lines.push("  -H 'Content-Type: application/json' \\");
    const json = JSON.stringify(endpoint.sample, null, 2).replaceAll("'", "'\\''");
    lines.push("  --data-raw '" + json + "'");
  }
  return (
    <VStack gap={3}>
      <DescriptionTable
        rows={[
          ["Authorization", "Bearer <access_token>; token cùng môi trường với URL."],
          ["Content-Type", endpoint.sample ? "application/json" : "Không cần với GET."],
          ["Accept-Language", "vi để yêu cầu nội dung tiếng Việt khi provider hỗ trợ."],
        ]}
      />
      <DocumentationCode code={lines.join("\n")} title="Ví dụ cURL" language="bash" width="100%" />
      <Text as="p" type="supporting" color="secondary">
        Đổi API_BASE_URL theo máy chủ của bạn và thay access token, fee_id hoặc Order ID trước khi
        gửi.
      </Text>
    </VStack>
  );
}

export function ErrorReference({ errors }) {
  return (
    <VStack gap={3} as="section" id="errors">
      <Heading level={2}>Lỗi và cách xử lý</Heading>
      <Text as="p" color="secondary">
        HTTP status cho biết loại lỗi; code trong JSON là mã chi tiết. Hai giá trị có thể khác nhau.
      </Text>
      <Table
        data={[...errors, ...commonErrors].map(([http, code, description, solution], index) => ({
          id: index,
          http,
          code,
          description,
          solution,
        }))}
        idKey="id"
        columns={[
          { key: "http", header: "HTTP" },
          { key: "code", header: "Code" },
          { key: "description", header: "Nguyên nhân" },
          { key: "solution", header: "Cách xử lý" },
        ]}
        verticalAlign="top"
        density="compact"
      />
    </VStack>
  );
}

export function RelatedDocuments({ ids }) {
  return (
    <VStack gap={3} as="section" id="next">
      <Heading level={2}>Bước tiếp theo</Heading>
      <HStack gap={3} wrap="wrap">
        {ids.map((id) => {
          const document = endpoints.find((endpoint) => endpoint.id === id) ?? guides[id];
          return (
            <Button key={id} label={document.title} href={"/docs/" + id} variant="secondary" />
          );
        })}
      </HStack>
    </VStack>
  );
}

export function GuideSections({ sections }) {
  return sections.map((section) => (
    <VStack key={section.id} as="section" id={section.id} gap={3}>
      <Heading level={2}>{section.title}</Heading>
      <DocumentationCopy paragraphs={section.paragraphs} />
      {section.rows && <DescriptionTable rows={section.rows} />}
    </VStack>
  ));
}

export function DocumentationCopy({ paragraphs }) {
  return (
    <VStack gap={2} className="doc-copy">
      {paragraphs.map((paragraph) => (
        <Text key={paragraph} as="p">
          {paragraph}
        </Text>
      ))}
    </VStack>
  );
}
