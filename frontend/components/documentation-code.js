"use client";

import { useState } from "react";
import { Button } from "@astryxdesign/core/Button";
import { CodeBlock } from "@astryxdesign/core/CodeBlock";
import { VStack } from "@astryxdesign/core/Stack";
import { useClipboard } from "@astryxdesign/core/hooks";

export default function DocumentationCode({ code, ...props }) {
  const { copy, isCopied } = useClipboard({ announce: "Đã sao chép" });
  const [failed, setFailed] = useState(false);
  const label = failed ? "Sao chép thất bại" : isCopied ? "Đã sao chép" : "Sao chép";
  return (
    <VStack className="doc-code">
      <CodeBlock {...props} code={code} hasCopyButton={false} />
      <Button
        label={label}
        size="sm"
        variant="ghost"
        className="doc-code-copy"
        tooltip={failed ? "Chọn đoạn code và sao chép thủ công." : undefined}
        onClick={async () => setFailed(!(await copy(code)))}
      />
    </VStack>
  );
}
