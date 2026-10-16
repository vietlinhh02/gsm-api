"use client";

import { useState, useSyncExternalStore } from "react";
import NextLink from "next/link";
import { AppShell } from "@astryxdesign/core/AppShell";
import { MobileNav, MobileNavToggle } from "@astryxdesign/core/MobileNav";
import { Theme } from "@astryxdesign/core";
import { Button } from "@astryxdesign/core/Button";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Link } from "@astryxdesign/core/Link";
import { Text } from "@astryxdesign/core/Text";
import { greensmTheme } from "@/themes/greensm.js";
import SiteIcon from "@/components/site-icon";

import DocsSidebar from "@/components/docs-sidebar";

const subscribeHydration = () => () => {};
const clientReady = () => true;
const serverReady = () => false;

function Header({ mode, setMode }) {
  return (
    <HStack
      as="header"
      className="site-header"
      gap={4}
      hAlign="between"
      vAlign="center"
      paddingInline={5}
      paddingBlock={3}
    >
      <HStack gap={2} vAlign="center">
        <MobileNavToggle label="Mở menu" />
        <Link href="/" as={NextLink} className="brand" color="primary">
          <HStack gap={2} vAlign="center">
            <SiteIcon name="car" color="accent" />
            <Text weight="semibold">GreenSM</Text>
            <Text color="secondary" className="brand-subtitle">
              API Docs
            </Text>
          </HStack>
        </Link>
      </HStack>
      <HStack gap={4} vAlign="center">
        <Link href="/" as={NextLink} color="primary" className="header-docs">
          <HStack gap={2} vAlign="center">
            <SiteIcon name="book-open" size="sm" />
            Tài liệu
          </HStack>
        </Link>
        <Link href="/playground" as={NextLink} color="primary" className="header-docs">
          <HStack gap={2} vAlign="center">
            <SiteIcon name="code" size="sm" />
            Playground
          </HStack>
        </Link>
        <Button
          label={mode === "light" ? "Giao diện tối" : "Giao diện sáng"}
          icon={<SiteIcon name={mode === "light" ? "moon" : "sun"} />}
          size="sm"
          variant="ghost"
          onClick={() => setMode(mode === "light" ? "dark" : "light")}
        />
        <Button
          href="/docs/auth"
          as={NextLink}
          label="Bắt đầu"
          icon={<SiteIcon name="arrow-right" />}
          variant="primary"
          size="sm"
          className="header-start"
        />
      </HStack>
    </HStack>
  );
}

export default function DocsShell({ children }) {
  const [mode, setMode] = useState("light");
  const ready = useSyncExternalStore(subscribeHydration, clientReady, serverReady);
  return (
    <Theme theme={greensmTheme} mode={mode}>
      <AppShell
        inert={!ready}
        height="auto"
        variant="surface"
        sideNav={<DocsSidebar />}
        topNav={<Header mode={mode} setMode={setMode} />}
        mobileNav={{
          breakpoint: "md",
          content: (
            <MobileNav header="Tài liệu">
              <DocsSidebar />
            </MobileNav>
          ),
        }}
      >
        <VStack className="page-frame">{children}</VStack>
      </AppShell>
    </Theme>
  );
}
