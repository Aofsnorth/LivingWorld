import net.minecraft.SharedConstants;
import net.minecraft.nbt.CompoundTag;
import net.minecraft.nbt.NbtOps;
import net.minecraft.server.Bootstrap;

import com.mojang.serialization.DataResult;

import java.io.FileInputStream;
import java.nio.file.Path;

/**
 * RegCheck — decodes the exact NBT bytes LivingWorld sends for one registry
 * entry using the real 26.3 codec, and prints the full failure.
 *
 * The Minecraft client crash report only says "Failed to parse value {...}",
 * naming the registry but not the offending field. Running the same bytes
 * through the vanilla codec surfaces DataResult's error, which carries the
 * offending path (e.g. "attributes.minecraft:gameplay/bed_rule.can_sleep").
 *
 * Usage: RegCheck <registry> <file.nbt>
 */
public class RegCheck {
    public static void main(String[] args) throws Exception {
        if (args.length < 2) {
            System.out.println("usage: RegCheck <registry> <file.nbt>");
            System.exit(2);
        }
        String registryName = args[0];
        Path file = Path.of(args[1]);

        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        CompoundTag tag;
        // The bytes we send are NETWORK nbt (little endian), so read them the
        // same way the game does, via FriendlyByteBuf.readNbt(), rather than
        // with the big-endian NbtIo reader.
        try (java.io.DataInputStream raw = new java.io.DataInputStream(new java.io.BufferedInputStream(new FileInputStream(file.toFile())))) {
            byte[] bytes = raw.readAllBytes();
            net.minecraft.network.FriendlyByteBuf buf =
                    new net.minecraft.network.FriendlyByteBuf(io.netty.buffer.Unpooled.wrappedBuffer(bytes));
            tag = buf.readNbt();
        }
        if (tag == null) {
            System.out.println("could not read NBT from " + file);
            System.exit(3);
        }

        System.out.println("read " + file + " -> keys=" + tag.keySet()
                + " has_skylight=" + tag.get("has_skylight"));
        // SNBT dump so our bytes can be diffed field-by-field against a
        // vanilla-encoded entry (see VanillaDump).
        if ("1".equals(System.getenv("REGCHECK_SNBT"))) {
            System.out.println("SNBT: " + tag);
        }
        Codec codec = Codec.forRegistry(registryName);
        if (codec == null) {
            System.out.println("NO CODEC wired for registry " + registryName);
            System.exit(3);
        }

        // Resolve Holders (timeline / world_clock / sound_id) against the real
        // vanilla registries, exactly like the game's registry loader does.
        //
        // REGCHECK_BARE=1 answers "no registry info" for every key, which
        // reproduces what the client sees if minecraft:world_clock /
        // minecraft:timeline are not bound yet when minecraft:dimension_type
        // is parsed. If the entry still decodes, ordering cannot be the bug.
        final var vanilla = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        final boolean bare = "1".equals(System.getenv("REGCHECK_BARE"));
        net.minecraft.resources.RegistryOps.RegistryInfoLookup info =
                new net.minecraft.resources.RegistryOps.RegistryInfoLookup() {
                    @Override
                    public <T> java.util.Optional<net.minecraft.core.HolderGetter<T>> lookup(
                            net.minecraft.resources.ResourceKey<? extends net.minecraft.core.Registry<? extends T>> key) {
                        return bare ? java.util.Optional.empty()
                                : vanilla.lookup(key).map(r -> (net.minecraft.core.HolderGetter<T>) r);
                    }
                };

        var ops = net.minecraft.resources.RegistryOps.create(NbtOps.INSTANCE, info);

        @SuppressWarnings({"rawtypes", "unchecked"})
        DataResult<?> result = ((com.mojang.serialization.Decoder) codec.codec).parse(ops, tag);
        if (result.result().isPresent()) {
            System.out.println("OK: " + registryName + " decoded");
        } else {
            System.out.println("FAIL: " + registryName);
            System.out.println("error: " + result.error().map(Object::toString).orElse("?"));
            System.exit(1);
        }
    }

    /** Maps a registry id onto the vanilla codec for its elements. */
    record Codec(com.mojang.serialization.Codec<?> codec) {
        static Codec forRegistry(String name) {
            // The client decodes each entry with RegistryData.elementCodec()
            // (see NetworkRegistryLoadTask.lambda$load$0). For most of these
            // types that is the *DIRECT_CODEC*; dimension_type additionally
            // has a NETWORK_CODEC, which is the one the networked registry
            // data uses. Using the plain "<Type>.CODEC" field is WRONG: that
            // is the Codec<Holder<T>> registry codec, and it misreports
            // failures.
            if (name.equals("minecraft:dimension_type")) {
                return new Codec(net.minecraft.world.level.dimension.DimensionType.NETWORK_CODEC);
            }
            if (name.equals("minecraft:trim_material")) {
                return new Codec(net.minecraft.world.item.equipment.trim.TrimMaterial.DIRECT_CODEC);
            }
            if (name.equals("minecraft:timeline")) {
                return new Codec(net.minecraft.world.timeline.Timeline.NETWORK_CODEC);
            }
            if (name.equals("minecraft:world_clock")) {
                return new Codec(net.minecraft.world.clock.WorldClock.DIRECT_CODEC);
            }
            return null;
        }
    }
}
