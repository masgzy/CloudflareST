#!/usr/bin/env python3
"""
iflow-cli → qoder-cli 一键配置迁移脚本
将 ~/.iflow 下的配置迁移至 ~/.qoder，兼容 macOS / Linux / Windows。
"""

import os
import sys
import re
import json
import shutil
from pathlib import Path
from datetime import datetime


def log(msg: str):
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{ts}] {msg}")


def migrate_file(src: Path, dst: Path):
    """复制单个文件，目标已存在则覆盖。"""
    if not src.exists():
        log(f"[跳过] 源文件不存在: {src}")
        return False
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(str(src), str(dst))
    log(f"[完成] {src} → {dst}")
    return True


def migrate_dir(src: Path, dst: Path):
    """复制整个文件夹，目标已存在则先删除再复制。"""
    if not src.exists():
        log(f"[跳过] 源目录不存在: {src}")
        return False
    if not src.is_dir():
        log(f"[跳过] 源路径不是目录: {src}")
        return False
    if dst.exists():
        shutil.rmtree(str(dst))
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(str(src), str(dst))
    log(f"[完成] {src} → {dst}")
    return True


def parse_toml_command(filepath: Path) -> dict:
    """简易解析 iflow commands 的 .toml 文件，提取 name、description、prompt。"""
    content = filepath.read_text(encoding="utf-8")
    result = {}

    # 从注释中提取 command name，如 "# Command: cleanproject"
    name_match = re.search(r"^#\s*Command:\s*(.+)$", content, re.MULTILINE)
    if name_match:
        result["name"] = name_match.group(1).strip()
    else:
        result["name"] = filepath.stem

    # 提取 description = "..."
    desc_match = re.search(r'^description\s*=\s*"([^"]*)"', content, re.MULTILINE)
    if desc_match:
        result["description"] = desc_match.group(1).strip()
    else:
        result["description"] = ""

    # 提取 prompt = """...""" (多行)
    prompt_match = re.search(r'^prompt\s*=\s*"""(.*?)"""', content, re.MULTILINE | re.DOTALL)
    if prompt_match:
        result["prompt"] = prompt_match.group(1).strip()
    else:
        # 尝试单行 prompt = "..."
        prompt_match = re.search(r'^prompt\s*=\s*"([^"]*)"', content, re.MULTILINE)
        result["prompt"] = prompt_match.group(1).strip() if prompt_match else ""

    return result


def convert_toml_to_md(parsed: dict) -> str:
    """将解析后的 toml command 转换为 qoder .md 格式。"""
    lines = []
    lines.append("---")
    lines.append(f"name: {parsed['name']}")
    lines.append(f"description: {parsed['description']}")
    lines.append("---")
    lines.append("")
    lines.append(parsed["prompt"])
    lines.append("")
    return "\n".join(lines)


def migrate_commands(src_dir: Path, dst_dir: Path) -> tuple:
    """将 ~/.iflow/commands/ 下的文件迁移到 ~/.qoder/commands/。
    .md 文件直接复制，.toml 文件转换为 .md 格式。"""
    if not src_dir.exists():
        log(f"[跳过] 源目录不存在: {src_dir}")
        return 0, 0, 0
    dst_dir.mkdir(parents=True, exist_ok=True)
    copied = 0
    converted = 0
    failed = 0
    # 直接复制 .md 文件
    for md_file in sorted(src_dir.glob("*.md")):
        try:
            dst_path = dst_dir / md_file.name
            shutil.copy2(str(md_file), str(dst_path))
            log(f"[复制] {md_file.name} → {dst_path.name}")
            copied += 1
        except Exception as e:
            log(f"[失败] {md_file.name}: {e}")
            failed += 1
    # 转换 .toml 文件为 .md
    for toml_file in sorted(src_dir.glob("*.toml")):
        try:
            parsed = parse_toml_command(toml_file)
            md_content = convert_toml_to_md(parsed)
            md_filename = parsed["name"] + ".md"
            dst_path = dst_dir / md_filename
            dst_path.write_text(md_content, encoding="utf-8")
            log(f"[转换] {toml_file.name} → {md_filename}")
            converted += 1
        except Exception as e:
            log(f"[失败] {toml_file.name}: {e}")
            failed += 1
    return copied, converted, failed


def migrate_mcp_servers(iflow_settings: Path, qoder_json: Path) -> bool:
    """将 ~/.iflow/settings.json 中的 mcpServers 合并到 ~/.qoder.json。"""
    if not iflow_settings.exists():
        log(f"[跳过] 源文件不存在: {iflow_settings}")
        return False

    with open(iflow_settings, "r", encoding="utf-8") as f:
        iflow_data = json.load(f)

    iflow_mcp = iflow_data.get("mcpServers")
    if not iflow_mcp:
        log("[跳过] ~/.iflow/settings.json 中没有 mcpServers 配置")
        return False

    # 读取目标文件（不存在则初始化空对象）
    if qoder_json.exists():
        with open(qoder_json, "r", encoding="utf-8") as f:
            qoder_data = json.load(f)
    else:
        qoder_data = {}

    # 合并 mcpServers：iflow 侧覆盖同名 server
    existing_mcp = qoder_data.get("mcpServers", {})
    merged_mcp = {**existing_mcp, **iflow_mcp}

    # 将 mcpServers 插入到 projects 之后
    # 通过重建有序字典来确保 key 顺序
    new_data = {}
    inserted = False
    for key, value in qoder_data.items():
        if key == "mcpServers":
            continue  # 跳过旧的，后面统一插入
        new_data[key] = value
        if key == "projects" and not inserted:
            new_data["mcpServers"] = merged_mcp
            inserted = True
    if not inserted:
        new_data["mcpServers"] = merged_mcp

    with open(qoder_json, "w", encoding="utf-8") as f:
        json.dump(new_data, f, indent=2, ensure_ascii=False)
        f.write("\n")

    added = set(iflow_mcp.keys()) - set(existing_mcp.keys())
    updated = set(iflow_mcp.keys()) & set(existing_mcp.keys())
    if added:
        log(f"[新增] mcpServers: {', '.join(sorted(added))}")
    if updated:
        log(f"[覆盖] mcpServers: {', '.join(sorted(updated))}")
    log(f"[完成] mcpServers 已合并到 {qoder_json}")
    return True


def migrate_agents(src_dir: Path, dst_dir: Path) -> bool:
    """将 agents 文件夹迁移到目标位置。目标已存在则合并（覆盖同名文件），不存在则整目录拷贝。"""
    if not src_dir.exists() or not src_dir.is_dir():
        log(f"[跳过] 源目录不存在: {src_dir}")
        return False
    if dst_dir.exists():
        # 目标已存在，逐个复制文件（覆盖同名）
        for item in src_dir.rglob("*"):
            rel = item.relative_to(src_dir)
            target = dst_dir / rel
            if item.is_dir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(str(item), str(target))
                log(f"[复制] {item} → {target}")
        log(f"[完成] agents 已合并到 {dst_dir}")
    else:
        dst_dir.parent.mkdir(parents=True, exist_ok=True)
        shutil.copytree(str(src_dir), str(dst_dir))
        log(f"[完成] {src_dir} → {dst_dir}")
    return True


def migrate_project_mcp_servers(iflow_settings: Path, mcp_json: Path) -> bool:
    """将项目级 .iflow/settings.json 中的 mcpServers 迁移到项目根目录的 .mcp.json。"""
    if not iflow_settings.exists():
        log(f"[跳过] 源文件不存在: {iflow_settings}")
        return False

    with open(iflow_settings, "r", encoding="utf-8") as f:
        iflow_data = json.load(f)

    iflow_mcp = iflow_data.get("mcpServers")
    if not iflow_mcp:
        log(f"[跳过] {iflow_settings} 中没有 mcpServers 配置")
        return False

    if mcp_json.exists():
        # 目标文件已存在，合并 mcpServers
        with open(mcp_json, "r", encoding="utf-8") as f:
            mcp_data = json.load(f)
        existing_mcp = mcp_data.get("mcpServers", {})
        merged_mcp = {**existing_mcp, **iflow_mcp}
        mcp_data["mcpServers"] = merged_mcp

        added = set(iflow_mcp.keys()) - set(existing_mcp.keys())
        updated = set(iflow_mcp.keys()) & set(existing_mcp.keys())
        if added:
            log(f"[新增] mcpServers: {', '.join(sorted(added))}")
        if updated:
            log(f"[覆盖] mcpServers: {', '.join(sorted(updated))}")
    else:
        # 目标文件不存在，直接创建
        mcp_data = {"mcpServers": iflow_mcp}
        log(f"[新建] {mcp_json}")

    with open(mcp_json, "w", encoding="utf-8") as f:
        json.dump(mcp_data, f, indent=2, ensure_ascii=False)
        f.write("\n")

    log(f"[完成] mcpServers 已迁移到 {mcp_json}")
    return True


def migrate_global():
    """执行全局配置迁移：~/.iflow → ~/.qoder。"""
    home = Path.home()
    iflow_dir = home / ".iflow"
    qoder_dir = home / ".qoder"

    log("===== 全局配置迁移开始 =====")
    log(f"源目录: {iflow_dir}")
    log(f"目标目录: {qoder_dir}")

    if not iflow_dir.exists():
        log(f"[错误] 源目录 {iflow_dir} 不存在，跳过全局迁移。")
        return

    qoder_dir.mkdir(parents=True, exist_ok=True)

    success = 0
    skipped = 0

    # 1. AGENTS.md（优先 AGENTS.md，其次 IFLOW.md）
    if (iflow_dir / "AGENTS.md").exists():
        if migrate_file(iflow_dir / "AGENTS.md", qoder_dir / "AGENTS.md"):
            success += 1
        else:
            skipped += 1
    elif (iflow_dir / "IFLOW.md").exists():
        log("[检测] 未找到 AGENTS.md，发现 IFLOW.md，将其迁移并重命名为 AGENTS.md")
        if migrate_file(iflow_dir / "IFLOW.md", qoder_dir / "AGENTS.md"):
            success += 1
        else:
            skipped += 1
    else:
        log("[跳过] AGENTS.md 和 IFLOW.md 均不存在")
        skipped += 1

    # 2. skills 文件夹
    if migrate_dir(iflow_dir / "skills", qoder_dir / "skills"):
        success += 1
    else:
        skipped += 1

    # 3. commands 迁移（.md 直接复制，.toml 转换为 .md）
    log("--- commands 迁移 ---")
    copied, converted, failed = migrate_commands(iflow_dir / "commands", qoder_dir / "commands")
    if copied > 0 or converted > 0:
        success += 1
    elif failed == 0:
        skipped += 1

    # 4. mcpServers: 从 settings.json 合并到 ~/.qoder.json
    log("--- mcpServers 合并 ---")
    if migrate_mcp_servers(iflow_dir / "settings.json", home / ".qoder.json"):
        success += 1
    else:
        skipped += 1

    # 5. agents 文件夹
    log("--- agents 迁移 ---")
    if migrate_agents(iflow_dir / "agents", qoder_dir / "agents"):
        success += 1
    else:
        skipped += 1

    log("===== 全局迁移结果 =====")
    log(f"成功: {success} 项 | 跳过: {skipped} 项 | 共计: {success + skipped} 项")
    if copied > 0 or converted > 0 or failed > 0:
        log(f"commands 迁移详情: 直接复制 {copied} 个, 转换 {converted} 个, 失败 {failed} 个")
    if skipped > 0:
        log("提示: 跳过的项目是因为源文件/目录不存在，属于正常情况。")
    log("===== 全局迁移完成 =====")


def migrate_project():
    """执行项目级配置迁移：<project>/.iflow → <project>/.qoder。"""
    project_dir = Path(__file__).resolve().parent
    iflow_dir = project_dir / ".iflow"
    qoder_dir = project_dir / ".qoder"

    log("===== 项目级配置迁移开始 =====")
    log(f"项目目录: {project_dir}")
    log(f"源目录: {iflow_dir}")
    log(f"目标目录: {qoder_dir}")

    if not iflow_dir.exists():
        log(f"[错误] 源目录 {iflow_dir} 不存在，跳过项目级迁移。")
        return

    qoder_dir.mkdir(parents=True, exist_ok=True)

    success = 0
    skipped = 0

    # 1. AGENTS.md：目标为 <project>/AGENTS.md
    agents_md = project_dir / "AGENTS.md"
    iflow_md = project_dir / "IFLOW.md"
    if agents_md.exists():
        log(f"[跳过] {agents_md} 已存在，无需迁移")
        skipped += 1
    elif iflow_md.exists():
        iflow_md.rename(agents_md)
        log(f"[重命名] {iflow_md} → {agents_md}")
        success += 1
    else:
        log("[跳过] 项目目录中 AGENTS.md 和 IFLOW.md 均不存在")
        skipped += 1

    # 2. skills 文件夹
    if migrate_dir(iflow_dir / "skills", qoder_dir / "skills"):
        success += 1
    else:
        skipped += 1

    # 3. commands 迁移（.md 直接复制，.toml 转换为 .md）
    log("--- commands 迁移 ---")
    copied, converted, failed = migrate_commands(iflow_dir / "commands", qoder_dir / "commands")
    if copied > 0 or converted > 0:
        success += 1
    elif failed == 0:
        skipped += 1

    # 4. mcpServers: 从 .iflow/settings.json 迁移到 <project>/.mcp.json
    log("--- mcpServers 迁移 ---")
    if migrate_project_mcp_servers(iflow_dir / "settings.json", project_dir / ".mcp.json"):
        success += 1
    else:
        skipped += 1

    # 5. agents 文件夹
    log("--- agents 迁移 ---")
    if migrate_agents(iflow_dir / "agents", qoder_dir / "agents"):
        success += 1
    else:
        skipped += 1

    log("===== 项目级迁移结果 =====")
    log(f"成功: {success} 项 | 跳过: {skipped} 项 | 共计: {success + skipped} 项")
    if copied > 0 or converted > 0 or failed > 0:
        log(f"commands 迁移详情: 直接复制 {copied} 个, 转换 {converted} 个, 失败 {failed} 个")
    if skipped > 0:
        log("提示: 跳过的项目是因为源文件/目录不存在，属于正常情况。")
    log("===== 项目级迁移完成 =====")


def main():
    log("===== iflow-cli → qoder-cli 配置迁移工具 =====")
    print()
    print("请选择迁移类型：")
    print("  1. 全局配置迁移（~/.iflow → ~/.qoder）")
    print("  2. 当前项目级配置迁移（<project>/.iflow → <project>/.qoder）")
    print("  3. 两者均迁移")
    print()

    while True:
        choice = input("请输入选项 (1/2/3): ").strip()
        if choice in ("1", "2", "3"):
            break
        print("无效输入，请输入 1、2 或 3。")

    print()

    if choice == "1":
        migrate_global()
    elif choice == "2":
        migrate_project()
    else:
        migrate_global()
        print()
        migrate_project()

    print()
    log("===== 所有迁移操作已完成 =====")


if __name__ == "__main__":
    main()
