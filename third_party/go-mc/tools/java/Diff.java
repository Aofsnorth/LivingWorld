import net.minecraft.nbt.CompoundTag;
import net.minecraft.nbt.ListTag;
import net.minecraft.nbt.Tag;

import java.io.FileInputStream;
import java.util.ArrayList;
import java.util.List;

/**
 * Diff — semantic field-by-field diff of two network-NBT files (nameless
 * TAG_Compound root). Prints every path where the values differ, regardless of
 * compound field order.
 *
 * Usage: Diff <a.nbt> <b.nbt>
 */
public class Diff {
    public static void main(String[] args) throws Exception {
        if (args.length < 2) {
            System.out.println("usage: Diff <a.nbt> <b.nbt>");
            System.exit(2);
        }
        CompoundTag a = read(args[0]);
        CompoundTag b = read(args[1]);
        int[] count = {0};
        diff("", a, b, count);
        System.out.println(count[0] == 0 ? "IDENTICAL" : count[0] + " differences");
    }

    static CompoundTag read(String file) throws Exception {
        byte[] bytes;
        try (FileInputStream in = new FileInputStream(file)) {
            bytes = in.readAllBytes();
        }
        var buf = new net.minecraft.network.FriendlyByteBuf(io.netty.buffer.Unpooled.wrappedBuffer(bytes));
        CompoundTag tag = buf.readNbt();
        if (tag == null) throw new IllegalStateException("no NBT in " + file);
        return tag;
    }

    static void diff(String path, Tag a, Tag b, int[] count) {
        if (a.getId() != b.getId()) {
            System.out.println("TYPE " + path + ": " + a.getId() + " (" + a + ") vs " + b.getId() + " (" + b + ")");
            count[0]++;
            return;
        }
        if (a instanceof CompoundTag ca && b instanceof CompoundTag cb) {
            List<String> keys = new ArrayList<>(ca.keySet());
            for (String k : cb.keySet()) if (!ca.contains(k)) keys.add(k);
            for (String k : keys) {
                String p = path + "." + k;
                if (!ca.contains(k)) { System.out.println("ONLY-B " + p + " = " + cb.get(k)); count[0]++; continue; }
                if (!cb.contains(k)) { System.out.println("ONLY-A " + p + " = " + ca.get(k)); count[0]++; continue; }
                diff(p, ca.get(k), cb.get(k), count);
            }
            return;
        }
        if (a instanceof ListTag la && b instanceof ListTag lb) {
            if (la.size() != lb.size()) {
                System.out.println("LEN " + path + ": " + la.size() + " (" + la + ") vs " + lb.size() + " (" + lb + ")");
                count[0]++;
                return;
            }
            for (int i = 0; i < la.size(); i++) diff(path + "[" + i + "]", la.get(i), lb.get(i), count);
            return;
        }
        if (!a.equals(b)) {
            System.out.println("VAL " + path + ": " + a + "  vs  " + b);
            count[0]++;
        }
    }
}
