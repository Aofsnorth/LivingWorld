import net.minecraft.SharedConstants;
import net.minecraft.core.registries.Registries;
import net.minecraft.nbt.CompoundTag;
import net.minecraft.nbt.NbtOps;
import net.minecraft.server.Bootstrap;
import net.minecraft.world.level.dimension.DimensionType;

import com.mojang.serialization.Dynamic;

import java.nio.file.Files;
import java.nio.file.Path;

/**
 * VanillaDump — encodes the overworld DimensionType exactly as a real vanilla
 * 26.3 server would (NbtOps) and writes it as network NBT (nameless
 * TAG_Compound root), so it can be diffed field-by-field against the bytes
 * LivingWorld sends.
 *
 * Usage: VanillaDump <out.nbt>
 */
public class VanillaDump {
    public static void main(String[] args) throws Exception {
        if (args.length < 1) {
            System.out.println("usage: VanillaDump <out.nbt>");
            System.exit(2);
        }
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        var ops = net.minecraft.resources.RegistryOps.create(NbtOps.INSTANCE, lookup);

        var key = net.minecraft.resources.ResourceKey.create(
                Registries.DIMENSION_TYPE,
                net.minecraft.resources.Identifier.parse("minecraft:overworld"));
        var dim = lookup.lookupOrThrow(Registries.DIMENSION_TYPE).get(key)
                .orElseThrow(() -> new IllegalStateException("no overworld dimension type"))
                .value();

        // NETWORK_CODEC is the element codec the registry loader uses over the
        // network (RegistryData.elementCodec()). The plain CODEC field is the
        // Holder codec and encodes to just a name string.
        var encoded = DimensionType.NETWORK_CODEC.encodeStart(ops, dim);
        if (encoded.result().isEmpty()) {
            System.out.println("ENCODE FAILED: " + encoded.error().map(Object::toString).orElse("?"));
            System.exit(1);
        }
        CompoundTag compound = (CompoundTag) (Object) encoded.result().get();

        var buf = new net.minecraft.network.FriendlyByteBuf(io.netty.buffer.Unpooled.buffer());
        buf.writeNbt(compound);
        byte[] bytes = new byte[buf.readableBytes()];
        buf.readBytes(bytes);
        Files.write(Path.of(args[0]), bytes);
        System.out.println("wrote " + bytes.length + " bytes");
    }
}
