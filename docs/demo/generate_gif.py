import os
import sys
import time
import subprocess
import shutil
import tempfile
import shlex
from PIL import Image, ImageDraw, ImageFont

# Canvas parameters
WIDTH = 950
HEIGHT = 520
FPS = 15
PADDING = 24
TOP_BAR_HEIGHT = 40

# Catppuccin Mocha Palette
BG_COLOR = (30, 30, 46)          # #1e1e2e Base
BAR_COLOR = (24, 24, 37)         # #181825 Mantle
TEXT_COLOR = (205, 214, 244)     # #cdd6f4 Text
SUBTEXT_COLOR = (166, 173, 200)  # #a6adc8 Subtext0
OVERLAY_COLOR = (108, 112, 134)  # #6c7086 Overlay0 (comments)
PROMPT_COLOR = (137, 180, 250)   # #89b4fa Blue
CMD_COLOR = (203, 166, 247)      # #cba6f7 Mauve
SUB_CMD_COLOR = (137, 220, 235)  # #89dceb Sky
ARG_COLOR = (166, 227, 161)      # #a6e3a1 Green
FLAG_COLOR = (250, 179, 135)     # #fab387 Peach
BORDER_COLOR = (88, 91, 112)     # #585b70 Surface2
HEADER_COLOR = (249, 226, 175)   # #f9e2af Yellow
SUCCESS_COLOR = (166, 227, 161)  # #a6e3a1 Green
CURSOR_COLOR = (245, 224, 220)   # #f5e0dc Rosewater

# Window control dots
RED_DOT = (243, 139, 168)
YELLOW_DOT = (249, 226, 175)
GREEN_DOT = (166, 227, 161)

def find_font():
    candidates = [
        r"C:\Windows\Fonts\DejaVuSansMono.ttf",
        r"C:\Windows\Fonts\LiberationMono-Regular.ttf",
        r"C:\Windows\Fonts\consola.ttf"
    ]
    for c in candidates:
        if os.path.exists(c):
            return c
    return None

FONT_PATH = find_font()
FONT_SIZE = 15
FONT = ImageFont.truetype(FONT_PATH, FONT_SIZE) if FONT_PATH else ImageFont.load_default()
FONT_BOLD = ImageFont.truetype(r"C:\Windows\Fonts\DejaVuSansMono-Bold.ttf", FONT_SIZE) if os.path.exists(r"C:\Windows\Fonts\DejaVuSansMono-Bold.ttf") else FONT
TITLE_FONT = ImageFont.truetype(FONT_PATH, 13) if FONT_PATH else FONT

LINE_HEIGHT = 21

def get_rowlite_binary():
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
    binary_path = os.path.join(root_dir, "rowlite.exe" if sys.platform == "win32" else "rowlite")
    if not os.path.exists(binary_path):
        print(f"Building {binary_path}...")
        subprocess.run(["go", "build", "-o", binary_path, "./cmd/rowlite"], cwd=root_dir, check=True)
    return binary_path

def create_base_terminal():
    img = Image.new("RGBA", (WIDTH, HEIGHT), BG_COLOR)
    draw = ImageDraw.Draw(img)
    
    # Top bar
    draw.rectangle([0, 0, WIDTH, TOP_BAR_HEIGHT], fill=BAR_COLOR)
    
    # Window controls
    dot_radius = 6
    y_center = TOP_BAR_HEIGHT // 2
    draw.ellipse([20, y_center - dot_radius, 20 + dot_radius*2, y_center + dot_radius], fill=RED_DOT)
    draw.ellipse([38, y_center - dot_radius, 38 + dot_radius*2, y_center + dot_radius], fill=YELLOW_DOT)
    draw.ellipse([56, y_center - dot_radius, 56 + dot_radius*2, y_center + dot_radius], fill=GREEN_DOT)
    
    # Title
    title = "rowlite — Terminal (950x520)"
    bbox = draw.textbbox((0, 0), title, font=TITLE_FONT)
    tw = bbox[2] - bbox[0]
    draw.text(((WIDTH - tw) // 2, (TOP_BAR_HEIGHT - (bbox[3] - bbox[1])) // 2), title, font=TITLE_FONT, fill=SUBTEXT_COLOR)
    
    # Border
    draw.rectangle([0, 0, WIDTH - 1, HEIGHT - 1], outline=BORDER_COLOR, width=1)
    return img

def render_screen(lines, cursor_pos=None):
    img = create_base_terminal()
    draw = ImageDraw.Draw(img)
    
    start_y = TOP_BAR_HEIGHT + 14
    start_x = PADDING
    
    for row_idx, line in enumerate(lines):
        curr_x = start_x
        curr_y = start_y + row_idx * LINE_HEIGHT
        
        if curr_y + LINE_HEIGHT > HEIGHT - 8:
            break
            
        for text, color, is_bold in line:
            f = FONT_BOLD if is_bold else FONT
            draw.text((curr_x, curr_y), text, font=f, fill=color)
            bbox = draw.textbbox((curr_x, curr_y), text, font=f)
            curr_x += (bbox[2] - bbox[0])
            
        if cursor_pos and cursor_pos[0] == row_idx:
            c_w = 9
            draw.rectangle([cursor_pos[1], curr_y + 2, cursor_pos[1] + c_w, curr_y + LINE_HEIGHT - 2], fill=CURSOR_COLOR)
            
    return img.convert("RGB")

def highlight_command(prompt_str, cmd_str):
    tokens = []
    tokens.append((prompt_str, PROMPT_COLOR, True))
    
    if not cmd_str:
        return tokens
        
    # Quote-aware lexer to keep arguments inside quotes together
    lexemes = []
    current = ""
    in_quote = None
    
    for ch in cmd_str:
        if ch in ('"', "'"):
            if in_quote == ch:
                in_quote = None
                current += ch
            elif in_quote is None:
                in_quote = ch
                current += ch
            else:
                current += ch
        elif ch == ' ' and in_quote is None:
            if current:
                lexemes.append(current)
                current = ""
            lexemes.append(' ')
        else:
            current += ch
            
    if current:
        lexemes.append(current)
        
    non_space_count = 0
    first_cmd = ""
    
    for lex in lexemes:
        if lex == ' ':
            tokens.append((' ', TEXT_COLOR, False))
            continue
            
        non_space_count += 1
        
        if non_space_count == 1:
            first_cmd = lex
            if lex == "rowlite":
                tokens.append((lex, CMD_COLOR, True))
            elif lex == "cat":
                tokens.append((lex, SUB_CMD_COLOR, True))
            else:
                tokens.append((lex, CMD_COLOR, True))
        elif non_space_count == 2 and first_cmd == "rowlite":
            tokens.append((lex, SUB_CMD_COLOR, True))
        elif lex.startswith("-"):
            tokens.append((lex, FLAG_COLOR, False))
        elif lex.startswith('"') or lex.startswith("'") or lex.endswith('"') or lex.endswith("'") or ("," in lex):
            tokens.append((lex, ARG_COLOR, False))
        else:
            tokens.append((lex, TEXT_COLOR, False))
            
    return tokens

def parse_output_line_colors(line_text):
    """Dynamically syntax highlights captured CLI output lines."""
    t = line_text
    if t.startswith("Successfully updated"):
        return [(t, SUCCESS_COLOR, True)]
    if t.startswith("---"):
        return [(t, OVERLAY_COLOR, False)]
    if t.startswith("rowlite:"):
        return [(t, SUB_CMD_COLOR, False)]
    if t.startswith("  audit:") or t.startswith("  autoincrement:"):
        return [(t, TEXT_COLOR, False)]
    if ":" in t and (t.strip().startswith("insert:") or t.strip().startswith("edit:") or t.strip().startswith("column:") or t.strip().startswith("step:")):
        k, v = t.split(":", 1)
        return [(k + ":", TEXT_COLOR, False), (v, ARG_COLOR, False)]
    
    # Table border line (Unicode or Markdown pipe)
    if "┌" in t or "├" in t or "└" in t:
        return [(t, BORDER_COLOR, False)]
    if t.startswith("|") and ("---" in t or "--" in t):
        return [(t, BORDER_COLOR, False)]
        
    # Table header line
    if "│" in t and (" id " in t or " task " in t or " status " in t):
        return [(t, HEADER_COLOR, True)]
    if t.startswith("|") and (" id " in t or " task " in t or " status " in t):
        return [(t, HEADER_COLOR, True)]
        
    return [(t, TEXT_COLOR, False)]

def run_real_command(rowlite_bin, command_line, sandbox_dir):
    """Executes the command in the sandbox directory and returns stdout string."""
    tokens = shlex.split(command_line)
    if tokens[0] == "rowlite":
        tokens[0] = rowlite_bin
        res = subprocess.run(tokens, cwd=sandbox_dir, capture_output=True, text=True)
        out = res.stdout
        if res.stderr:
            out += res.stderr
        return out.strip()
    elif tokens[0] == "cat":
        target_file = os.path.join(sandbox_dir, tokens[1])
        if os.path.exists(target_file):
            with open(target_file, "r", encoding="utf-8") as f:
                return f.read().strip()
        return ""
    return ""

def generate_frames():
    rowlite_bin = get_rowlite_binary()
    sandbox_dir = tempfile.mkdtemp(prefix="rowlite_demo_")
    
    try:
        frames_part1 = []
        frames_part2 = []
        target_frames = frames_part1
        current_lines = []
        
        def add_blank(n=1):
            for _ in range(n):
                current_lines.append([("", TEXT_COLOR, False)])
                
        def type_command(comment, cmd_full, duration_frames=24, hold_comment=8, hold_after=12, pre_exec_sleep=0.0):
            current_lines.append([(comment, OVERLAY_COLOR, False)])
            frame = render_screen(current_lines)
            for _ in range(hold_comment):
                target_frames.append(frame)
            
            prompt = "❯ "
            f_temp = ImageDraw.Draw(Image.new("RGBA", (10, 10)))
            prompt_w = f_temp.textbbox((0, 0), prompt, font=FONT_BOLD)[2]
            
            total_len = len(cmd_full)
            step_size = max(1, total_len // duration_frames)
            
            for char_idx in range(1, total_len + 1, step_size):
                partial_cmd = cmd_full[:char_idx]
                hl = highlight_command(prompt, partial_cmd)
                
                drawn_w = prompt_w
                for t, _, is_b in hl[1:]:
                    drawn_w += f_temp.textbbox((0, 0), t, font=FONT_BOLD if is_b else FONT)[2]
                    
                cursor_x = PADDING + drawn_w
                line_copy = list(current_lines)
                line_copy.append(hl)
                frame = render_screen(line_copy, cursor_pos=(len(current_lines), cursor_x))
                target_frames.append(frame)
                
            hl_final = highlight_command(prompt, cmd_full)
            current_lines.append(hl_final)
            frame = render_screen(current_lines)
            for _ in range(hold_after):
                target_frames.append(frame)
                
            if pre_exec_sleep > 0:
                time.sleep(pre_exec_sleep)
                
            # Execute real command and capture live output
            real_output = run_real_command(rowlite_bin, cmd_full, sandbox_dir)
            if real_output:
                output_lines = real_output.splitlines()
                for line in output_lines:
                    colored_line = parse_output_line_colors(line)
                    current_lines.append(colored_line)
                frame = render_screen(current_lines)
                target_frames.append(frame)
                
            return real_output
            
        # ==========================================
        # PART 1: Init, Append, and List (Screen 1)
        # ==========================================
        target_frames = frames_part1
        
        # Initial pause (1.0s)
        for _ in range(15):
            target_frames.append(render_screen([([("❯ ", PROMPT_COLOR, True)])], cursor_pos=(0, PADDING + 16)))
        
        # 1. Step 1: Init table with explicit columns and autoincrement with --audit
        type_command(
            "# Step 1: Initialize table schema with autoincrement ID & audit logging",
            'rowlite init tasks.md --columns "id,task,status,priority" --autoincrement "id" --audit',
            duration_frames=32,
            hold_comment=10,
            hold_after=14
        )
        add_blank(1)
        
        # 2. Step 2: Append 2 rows
        type_command(
            "# Step 2: Append rows (automatic ID sequence & timestamps)",
            'rowlite append tasks.md ",Implement MCP protocol,pending,high"',
            duration_frames=24,
            hold_comment=8,
            hold_after=12
        )
        type_command(
            "# Append next row",
            'rowlite append tasks.md ",Record hero demo GIF,in_progress,medium"',
            duration_frames=24,
            hold_comment=6,
            hold_after=14
        )
        add_blank(1)
        
        # 3. Step 3: Query table with clean Unicode formatting
        type_command(
            "# Step 3: Query table with clean Unicode formatting",
            'rowlite list tasks.md --format pretty',
            duration_frames=18,
            hold_comment=8,
            hold_after=10
        )
        
        # Pause on live table view (4.0s for Part 1)
        frame = render_screen(current_lines)
        for _ in range(60):
            target_frames.append(frame)
            
        # ==========================================
        # PART 2: Update and Cat (Screen 2)
        # ==========================================
        target_frames = frames_part2
        current_lines = []
        
        # Initial pause for Part 2 (1.0s)
        for _ in range(15):
            target_frames.append(render_screen([([("❯ ", PROMPT_COLOR, True)])], cursor_pos=(0, PADDING + 16)))
        
        # 4. Step 4: Update row (invokes real rowlite update with real wait time for clear timestamp mutation)
        type_command(
            "# Step 4: Precisely update row fields in-place",
            'rowlite update tasks.md --where "id == 1" --set "status=completed"',
            duration_frames=26,
            hold_comment=8,
            hold_after=14,
            pre_exec_sleep=4.0
        )
        
        frame = render_screen(current_lines)
        for _ in range(18): # 1.2s pause after update
            target_frames.append(frame)
        add_blank(1)
        
        # 5. Step 5: Verify on disk (reads actual file from disk)
        type_command(
            "# Step 5: Verify auto-formatted Markdown table on disk",
            'cat tasks.md',
            duration_frames=12,
            hold_comment=8,
            hold_after=8
        )
        
        # Pause at end to inspect Markdown table (4.3s)
        frame = render_screen(current_lines)
        for _ in range(65):
            target_frames.append(frame)
            
        return frames_part1, frames_part2
    finally:
        shutil.rmtree(sandbox_dir, ignore_errors=True)

def save_gif_from_frames(frames, output_path):
    temp_dir = tempfile.mkdtemp()
    try:
        for idx, f in enumerate(frames):
            frame_path = os.path.join(temp_dir, f"frame_{idx:04d}.png")
            f.save(frame_path)
            
        palette_path = os.path.join(temp_dir, "palette.png")
        cmd_palette = [
            "ffmpeg", "-y",
            "-framerate", str(FPS),
            "-i", os.path.join(temp_dir, "frame_%04d.png"),
            "-vf", "palettegen=stats_mode=diff",
            palette_path
        ]
        subprocess.run(cmd_palette, check=True)
        
        cmd_gif = [
            "ffmpeg", "-y",
            "-framerate", str(FPS),
            "-i", os.path.join(temp_dir, "frame_%04d.png"),
            "-i", palette_path,
            "-lavfi", "paletteuse=dither=bayer:bayer_scale=3",
            output_path
        ]
        subprocess.run(cmd_gif, check=True)
        
        size_kb = os.path.getsize(output_path) / 1024
        print(f"Successfully generated {output_path} ({size_kb:.1f} KB, {len(frames)/FPS:.1f}s)")
    finally:
        shutil.rmtree(temp_dir, ignore_errors=True)

def build_gif():
    frames_p1, frames_p2 = generate_frames()
    frames_full = frames_p1 + frames_p2
    
    assets_dir = r"c:\work\git\rowlite\docs\assets"
    os.makedirs(assets_dir, exist_ok=True)
    
    # 1. Full combined demo
    save_gif_from_frames(frames_full, os.path.join(assets_dir, "rowlite-demo.gif"))
    
    # 2. Part 1 (Screen 1): Init, Append, and List
    save_gif_from_frames(frames_p1, os.path.join(assets_dir, "rowlite-demo-part1.gif"))
    
    # 3. Part 2 (Screen 2): Update and Cat
    save_gif_from_frames(frames_p2, os.path.join(assets_dir, "rowlite-demo-part2.gif"))

if __name__ == "__main__":
    build_gif()
