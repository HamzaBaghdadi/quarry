"use client";

import { useEffect, useState } from "react";
import { ArrowsClockwise, Command, GearSix, GithubLogo, Info, Moon, Sun } from "@phosphor-icons/react";
import { toast } from "sonner";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { api } from "@/lib/api";
import { win } from "@/lib/platform";
import { useStore } from "@/lib/store";
import { useUpdater } from "@/lib/updater";
import { mod } from "./common";

const item = "cursor-pointer gap-2.5 text-[13px] [&_svg]:size-4 [&_svg]:text-fg-muted";

export function SettingsMenu() {
  const theme = useStore((s) => s.theme);
  const { toggleTheme, setPalette } = useStore.getState();
  const updater = useUpdater();
  const [version, setVersion] = useState("");

  useEffect(() => {
    void api.version().then(setVersion);
  }, []);

  const checkNow = async () => {
    await updater.check(true);
    const s = useUpdater.getState();
    if (s.error) toast.error("Couldn't check for updates", { description: s.error });
    else if (!s.available) toast.success("You're on the latest version", { description: `Quarry ${version}` });
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Settings"
        className="relative flex size-9 cursor-pointer items-center justify-center rounded-lg text-fg-muted transition-colors duration-150 hover:bg-bg-2 hover:text-fg data-[state=open]:bg-bg-3 data-[state=open]:text-fg"
      >
        <GearSix className="size-[18px]" />
        {updater.available && <span aria-label="Update available" className="absolute right-1.5 top-1.5 size-2 rounded-full bg-brand" />}
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="end" sideOffset={8} className="w-60 border-line-strong bg-popover">
        <DropdownMenuLabel className="text-[11px] font-normal text-fg-subtle">Quarry {version}</DropdownMenuLabel>
        <DropdownMenuItem className={item} onSelect={() => setPalette(true)}>
          <Command /> Command palette
          <DropdownMenuShortcut>{mod()} K</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem className={item} onSelect={toggleTheme}>
          {theme === "dark" ? <Sun /> : <Moon />} {theme === "dark" ? "Light theme" : "Dark theme"}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {updater.available ? (
          <DropdownMenuItem className={item} onSelect={updater.openDialog}>
            <ArrowsClockwise className="!text-brand-text" /> Update to {updater.available.version}
          </DropdownMenuItem>
        ) : (
          <DropdownMenuItem className={item} disabled={updater.status === "checking"} onSelect={(e) => (e.preventDefault(), void checkNow())}>
            <ArrowsClockwise /> {updater.status === "checking" ? "Checking…" : "Check for updates"}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem className={item} onSelect={() => win.openURL("https://github.com/HamzaBaghdadi/quarry")}>
          <GithubLogo /> GitHub repository
        </DropdownMenuItem>
        <DropdownMenuItem className={item} onSelect={() => win.openURL("https://github.com/HamzaBaghdadi/quarry/releases")}>
          <Info /> Release notes
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
