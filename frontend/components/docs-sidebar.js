"use client";

import { useEffect, useState } from "react";
import NextLink from "next/link";
import { usePathname } from "next/navigation";
import { useAppShellMobile } from "@astryxdesign/core/AppShell";
import { Icon } from "@astryxdesign/core/Icon";
import { Link } from "@astryxdesign/core/Link";
import { SideNav, SideNavHeading, SideNavItem, SideNavSection } from "@astryxdesign/core/SideNav";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { TextInput } from "@astryxdesign/core/TextInput";
import { iconTypes, pageIcons } from "@/components/site-icon";
import { navigation } from "#lib/catalog.mjs";

function normalize(value) {
  return value
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .replace(/đ/gi, "d")
    .toLowerCase();
}

function ApiStatus() {
  const [status, setStatus] = useState("Đang kiểm tra");
  useEffect(() => {
    const controller = new AbortController();
    fetch("/api/backend/healthz", { signal: controller.signal })
      .then((response) => setStatus(response.ok ? "API sẵn sàng" : "API chưa kết nối"))
      .catch((error) => {
        if (error.name !== "AbortError") setStatus("API chưa kết nối");
      });
    return () => controller.abort();
  }, []);
  return (
    <HStack gap={2} vAlign="center">
      <Icon
        icon={status === "API sẵn sàng" ? "success" : "info"}
        size="xsm"
        color={status === "API sẵn sàng" ? "success" : "secondary"}
      />
      <Text type="supporting">{status}</Text>
    </HStack>
  );
}

function NavigationItems({ items, pathname, onNavigate }) {
  return items.map((item) => (
    <SideNavItem
      key={item.id}
      label={item.title}
      icon={iconTypes[pageIcons[item.id]]}
      href={item.href}
      as={NextLink}
      onClick={onNavigate}
      isSelected={pathname === item.href}
      endContent={
        item.method && (
          <Text type="supporting" color="secondary">
            {item.method}
          </Text>
        )
      }
    />
  ));
}

export default function DocsSidebar() {
  const pathname = usePathname();
  const { closeMobileNav } = useAppShellMobile();
  const [query, setQuery] = useState("");
  const groups = navigation
    .map((group) => ({
      ...group,
      items: group.items.filter((item) =>
        normalize(item.title + " " + item.id + " " + (item.method ?? "")).includes(
          normalize(query),
        ),
      ),
    }))
    .filter((group) => group.items.length > 0);
  return (
    <SideNav
      className="docs-sidebar"
      header={<SideNavHeading heading="GreenSM API" />}
      topContent={
        <TextInput
          label="Tìm tài liệu"
          isLabelHidden
          placeholder="Tìm tài liệu…"
          startIcon="search"
          value={query}
          onChange={setQuery}
          hasClear
          width="100%"
        />
      }
      footer={
        <VStack gap={2} padding={4}>
          <ApiStatus />
          <Link href="https://astryx.atmeta.com/" target="_blank" type="supporting">
            Giao diện Astryx
          </Link>
        </VStack>
      }
    >
      {groups.map((group) => (
        <SideNavSection key={group.title} title={group.title}>
          <NavigationItems items={group.items} pathname={pathname} onNavigate={closeMobileNav} />
        </SideNavSection>
      ))}
      {groups.length === 0 && (
        <Text as="p" color="secondary">
          Không tìm thấy tài liệu phù hợp.
        </Text>
      )}
    </SideNav>
  );
}
